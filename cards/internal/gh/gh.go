// Package gh is a small GitHub API client with an in-memory data cache. The
// gateway caches rendered cards per URL; this caches the data underneath, so
// different themes or layouts of the same card cost one API call.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Error is a failure to show on the card, with the HTTP status to send.
type Error struct {
	Message string
	Status  int
}

func (e *Error) Error() string { return e.Message }

func errorf(status int, msg string) *Error { return &Error{msg, status} }

type Client struct {
	BaseURL string // https://api.github.com
	Token   string
	HTTP    *http.Client
}

func New(token string) *Client {
	return &Client{BaseURL: "https://api.github.com", Token: token, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) do(req *http.Request) (*http.Response, []byte, error) {
	req.Header.Set("Authorization", "bearer "+c.Token)
	req.Header.Set("User-Agent", "github-stats-cards")
	res, err := c.HTTP.Do(req)
	if err != nil {
		log.Printf("github request failed: %v", err)
		return nil, nil, errorf(http.StatusBadGateway, "Could not load data from GitHub")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	return res, body, err
}

// GraphQL runs a query and decodes its data into out.
func (c *Client) GraphQL(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/graphql", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res, raw, err := c.do(req)
	if err != nil {
		return err
	}
	var body struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(raw, &body)
	if res.StatusCode != http.StatusOK || len(body.Errors) > 0 || len(body.Data) == 0 || string(body.Data) == "null" {
		log.Printf("graphql failed: %d %s", res.StatusCode, truncate(raw, 500))
		switch {
		case len(body.Errors) > 0 && body.Errors[0].Type == "NOT_FOUND":
			return errorf(http.StatusNotFound, "Could not find that user")
		case len(body.Errors) > 0 && body.Errors[0].Type == "RATE_LIMITED", res.StatusCode == 403, res.StatusCode == 429:
			return errorf(http.StatusBadGateway, "GitHub API rate limit reached")
		case res.StatusCode == 401:
			return errorf(http.StatusBadGateway, "GitHub token is invalid or expired")
		}
		return errorf(http.StatusBadGateway, "Could not load data from GitHub")
	}
	if err := json.Unmarshal(body.Data, out); err != nil {
		return fmt.Errorf("decoding graphql data: %w", err)
	}
	return nil
}

// REST GETs path and decodes the JSON response into out.
func (c *Client) REST(ctx context.Context, path string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, raw, err := c.do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		log.Printf("rest %s failed: %d %s", path, res.StatusCode, truncate(raw, 500))
		if res.StatusCode == 401 {
			return errorf(http.StatusBadGateway, "GitHub token is invalid or expired")
		}
		return errorf(http.StatusBadGateway, "Could not load data from GitHub")
	}
	return json.Unmarshal(raw, out)
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
