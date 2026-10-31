// fakegh stands in for api.github.com during benchmarks, answering each query
// the cards make with fixed data (the same fixtures as the cards tests).
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func calendarDays(from, to string) []map[string]any {
	end := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	var out []map[string]any
	for i := 399; i >= 0; i-- {
		date := end.AddDate(0, 0, -i).Format(time.DateOnly)
		if date < from[:10] || date > to[:10] {
			continue
		}
		n := (i % 7) + 1
		switch {
		case i < 5:
			n = 2 + i
		case i%3 == 0:
			n = 0
		}
		out = append(out, map[string]any{"date": date, "contributionCount": n})
	}
	return out
}

type obj = map[string]any

func weeks(days []obj) []any { return []any{obj{"contributionDays": days}} }

func pr(title, state, repo, owner, perm string, stars int) obj {
	return obj{"title": title, "url": "u", "state": state, "createdAt": "", "mergedAt": nil, "repository": obj{
		"nameWithOwner": repo, "isPrivate": false, "stargazerCount": stars, "viewerPermission": perm, "owner": obj{"login": owner},
	}}
}

func answer(query string, v obj) any {
	count := func(n int) obj { return obj{"totalCount": n} }
	switch {
	case strings.Contains(query, "totalCommitContributions"):
		return obj{"user": obj{
			"name": "Fu San", "login": "FuSan21",
			"commits":                      obj{"totalCommitContributions": 812},
			"reviews":                      obj{"totalPullRequestReviewContributions": 14},
			"repositoriesContributedTo":    count(23),
			"pullRequests":                 count(120),
			"mergedPullRequests":           count(100),
			"openIssues":                   count(4),
			"closedIssues":                 count(30),
			"followers":                    count(40),
			"repositoryDiscussions":        count(2),
			"repositoryDiscussionComments": count(1),
			"repositories": obj{
				"nodes":    []any{obj{"name": "big", "stargazers": count(1500)}, obj{"name": "small", "stargazers": count(0)}},
				"pageInfo": obj{"hasNextPage": false, "endCursor": nil},
			},
		}}
	case strings.Contains(query, "languages("):
		lang := func(name string, color any, size int) any {
			return obj{"size": size, "node": obj{"name": name, "color": color}}
		}
		repo := func(name string, langs ...any) any { return obj{"name": name, "languages": obj{"edges": langs}} }
		return obj{"user": obj{"repositories": obj{
			"nodes": []any{
				repo("big", lang("TypeScript", "#3178c6", 6000), lang("CSS", "#663399", 1000)),
				repo("small", lang("Kotlin", "#A97BFF", 3000), lang("TypeScript", "#3178c6", 500)),
				repo("other", lang("Dockerfile", nil, 2000), lang("Go", "#00ADD8", 2500)),
			},
			"pageInfo": obj{"hasNextPage": false, "endCursor": nil},
		}}}
	case strings.Contains(query, "contributionYears"):
		return obj{"user": obj{
			"createdAt": "2025-03-01T00:00:00Z",
			"contributionsCollection": obj{
				"contributionYears":    []int{2026, 2025},
				"contributionCalendar": obj{"weeks": weeks(calendarDays(v["from"].(string), v["to"].(string)))},
			},
		}}
	case strings.Contains(query, "contributionCalendar"):
		return obj{"user": obj{"name": "Fu San", "contributionsCollection": obj{
			"contributionCalendar": obj{"weeks": weeks(calendarDays(v["from"].(string), v["to"].(string)))},
		}}}
	case strings.Contains(query, "search("):
		return obj{"search": obj{"nodes": []any{
			pr("Fix crash on <empty> input", "MERGED", "acme/lib", "acme", "READ", 1234),
			pr("Add feature", "OPEN", "acme/app", "acme", "READ", 0),
			pr("Improve docs", "MERGED", "other/tool", "other", "READ", 87),
		}}}
	}
	return nil
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/commits") {
			_, _ = io.WriteString(w, `{"total_count":4321}`)
			return
		}
		var req struct {
			Query     string `json:"query"`
			Variables obj    `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj{"data": answer(req.Query, req.Variables)})
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
