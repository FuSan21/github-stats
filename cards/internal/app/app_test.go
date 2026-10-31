package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FuSan21/github-stats/cards/internal/cards"
	"github.com/FuSan21/github-stats/cards/internal/gh"
	"github.com/FuSan21/github-stats/cards/internal/render"
)

const user = "FuSan21"

var now = time.Date(2026, 10, 31, 6, 0, 0, 0, time.UTC)

// fakeGitHub answers each query the cards make with fixed data.
type fakeGitHub struct {
	calls    atomic.Int32
	failNext atomic.Int32
}

// 400 days of contributions ending 2026-10-31: every 3rd day empty, the last 5 days all active.
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

func weeks(days []map[string]any) []any { return []any{map[string]any{"contributionDays": days}} }

func repoPR(title, state, repo, owner, perm string, stars int) map[string]any {
	return map[string]any{"title": title, "url": "u", "state": state, "repository": map[string]any{
		"nameWithOwner": repo, "isPrivate": false, "stargazerCount": stars, "viewerPermission": perm, "owner": map[string]any{"login": owner},
	}}
}

func (f *fakeGitHub) answer(query string, v map[string]any) any {
	switch {
	case strings.Contains(query, "totalCommitContributions"):
		return map[string]any{"user": map[string]any{
			"name": "Fu San", "login": user,
			"commits":                      map[string]any{"totalCommitContributions": 812},
			"reviews":                      map[string]any{"totalPullRequestReviewContributions": 14},
			"repositoriesContributedTo":    map[string]any{"totalCount": 23},
			"pullRequests":                 map[string]any{"totalCount": 120},
			"mergedPullRequests":           map[string]any{"totalCount": 100},
			"openIssues":                   map[string]any{"totalCount": 4},
			"closedIssues":                 map[string]any{"totalCount": 30},
			"followers":                    map[string]any{"totalCount": 40},
			"repositoryDiscussions":        map[string]any{"totalCount": 2},
			"repositoryDiscussionComments": map[string]any{"totalCount": 1},
			"repositories": map[string]any{
				"nodes":    []any{map[string]any{"name": "big", "stargazers": map[string]any{"totalCount": 1500}}, map[string]any{"name": "small", "stargazers": map[string]any{"totalCount": 30}}},
				"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "c1"},
			},
		}}
	case strings.Contains(query, "stargazers") && v["after"] == "c1":
		return map[string]any{"user": map[string]any{"repositories": map[string]any{
			"nodes":    []any{map[string]any{"name": "tiny", "stargazers": map[string]any{"totalCount": 5}}, map[string]any{"name": "none", "stargazers": map[string]any{"totalCount": 0}}},
			"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "c2"},
		}}}
	case strings.Contains(query, "languages("):
		lang := func(name string, color any, size int) any {
			return map[string]any{"size": size, "node": map[string]any{"name": name, "color": color}}
		}
		repo := func(name string, langs ...any) any {
			return map[string]any{"name": name, "languages": map[string]any{"edges": langs}}
		}
		if v["after"] == nil {
			return map[string]any{"user": map[string]any{"repositories": map[string]any{
				"nodes": []any{
					repo("big", lang("TypeScript", "#3178c6", 6000), lang("CSS", "#663399", 1000)),
					repo("small", lang("Kotlin", "#A97BFF", 3000), lang("TypeScript", "#3178c6", 500)),
				},
				"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "p2"},
			}}}
		}
		return map[string]any{"user": map[string]any{"repositories": map[string]any{
			"nodes":    []any{repo("other", lang("Dockerfile", nil, 2000))},
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
		}}}
	case strings.Contains(query, "contributionYears"):
		return map[string]any{"user": map[string]any{
			"createdAt": "2025-03-01T00:00:00Z",
			"contributionsCollection": map[string]any{
				"contributionYears":    []int{2026, 2025},
				"contributionCalendar": map[string]any{"weeks": weeks(calendarDays(v["from"].(string), v["to"].(string)))},
			},
		}}
	case strings.Contains(query, "contributionCalendar"):
		return map[string]any{"user": map[string]any{"name": "Fu San", "contributionsCollection": map[string]any{
			"contributionCalendar": map[string]any{"weeks": weeks(calendarDays(v["from"].(string), v["to"].(string)))},
		}}}
	case strings.Contains(query, "search("):
		return map[string]any{"search": map[string]any{"nodes": []any{
			repoPR("Fix crash on <empty> input", "MERGED", "acme/lib", "acme", "READ", 1234),
			repoPR("Add feature", "OPEN", "acme/app", "acme", "READ", 0),
			repoPR("Work repo", "MERGED", "employer/svc", "employer", "WRITE", 3),
			repoPR("Rejected", "CLOSED", "acme/lib", "acme", "READ", 1234),
		}}}
	}
	panic("unexpected query: " + query[:min(80, len(query))])
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	if f.failNext.Load() > 0 {
		f.failNext.Add(-1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"message":"Server Error"}`)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/search/commits") {
		_, _ = io.WriteString(w, `{"total_count":4321}`)
		return
	}
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": f.answer(req.Query, req.Variables)})
}

func setup(t *testing.T) (*App, *fakeGitHub) {
	t.Helper()
	fake := &fakeGitHub{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client := gh.New("test-token")
	client.BaseURL = srv.URL
	views := cards.LoadViews(filepath.Join(t.TempDir(), "views.json"), 1718)
	return &App{User: user, GitHub: client, Cache: gh.NewCache(), Views: views, Now: func() time.Time { return now }}, fake
}

func get(t *testing.T, a *App, path string) (int, http.Header, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Header(), rec.Body.String()
}

func TestRoutes(t *testing.T) {
	a, _ := setup(t)
	for path, want := range map[string]string{
		"/api?username=FuSan21&theme=gruvbox&show_icons=true":           "GitHub Stats",
		"/api/top-langs?username=FuSan21&layout=compact&theme=gruvbox":  "Most Used Languages",
		"/streak?user=FuSan21&theme=gruvbox&hide_border=true":           "Current Streak",
		"/graph?username=FuSan21&theme=gruvbox":                         "Contribution Graph",
		"/oss?username=FuSan21&theme=gruvbox":                           "Open Source Contributions",
		"/views?username=FuSan21":                                       "Profile views",
		"/api/top-langs?username=FuSan21&layout=donut-vertical&theme=x": "Kotlin",
	} {
		t.Run(path, func(t *testing.T) {
			status, h, body := get(t, a, path)
			if status != http.StatusOK || !strings.Contains(h.Get("Content-Type"), "image/svg+xml") {
				t.Fatalf("status %d, type %q: %s", status, h.Get("Content-Type"), body)
			}
			if err := render.CheckWellFormed(body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, want) {
				t.Errorf("missing %q", want)
			}
		})
	}
}

func TestAccess(t *testing.T) {
	a, _ := setup(t)
	for path, want := range map[string]int{
		"/api?username=fusan21":           200, // usernames are case-insensitive
		"/api?username=someone":           403,
		"/api":                            403,
		"/streak?username=FuSan21":        403, // /streak takes `user`
		"/api/pin?username=FuSan21":       404,
		"/views?username=someone":         403,
		"/graph?username=FuSan21&days=-3": 200,
	} {
		if status, _, _ := get(t, a, path); status != want {
			t.Errorf("%s: %d, want %d", path, status, want)
		}
	}
	if _, _, body := get(t, a, "/healthz"); body != "ok\n" {
		t.Errorf("healthz: %q", body)
	}
}

func TestData(t *testing.T) {
	a, fake := setup(t)

	// Stats page through starred repos and sum their stars.
	_, _, body := get(t, a, "/api?username=FuSan21&number_format=long")
	if !strings.Contains(body, ">1535<") || fake.calls.Load() != 2 {
		t.Errorf("stars: calls=%d", fake.calls.Load())
	}
	_, _, body = get(t, a, "/api?username=FuSan21&include_all_commits=true&number_format=long")
	if !strings.Contains(body, ">4321<") || !strings.Contains(body, "Total Commits:") {
		t.Error("include_all_commits not applied")
	}

	// Top languages read every page; themes and layouts share the cached data.
	fake.calls.Store(0)
	_, _, body = get(t, a, "/api/top-langs?username=FuSan21&layout=compact&langs_count=10")
	get(t, a, "/api/top-langs?username=FuSan21&layout=donut&theme=dark")
	if !strings.Contains(body, "Dockerfile") || !strings.Contains(body, "TypeScript 52.00%") || fake.calls.Load() != 2 {
		t.Errorf("top langs: calls=%d", fake.calls.Load())
	}

	// OSS drops repos with push access and closed PRs.
	_, _, body = get(t, a, "/oss?username=FuSan21")
	for _, want := range []string{"acme/lib", "Fix crash on &lt;empty&gt; input", "1 merged PR across 1 project"} {
		if !strings.Contains(body, want) {
			t.Errorf("oss missing %q", want)
		}
	}
	if strings.Contains(body, "employer/svc") || strings.Contains(body, "Rejected") {
		t.Error("oss shows a repo with push access or a closed PR")
	}

	// Streak fetches each year since the account was created: 2026 (for createdAt), then 2025.
	fake.calls.Store(0)
	_, _, body = get(t, a, "/streak?user=FuSan21")
	if fake.calls.Load() != 2 || !strings.Contains(body, "Current Streak") {
		t.Errorf("streak: calls=%d", fake.calls.Load())
	}

	// The graph ends today (Oct 31 in the fixture), 31 days by default.
	_, _, body = get(t, a, "/graph?username=FuSan21")
	if n := strings.Count(body, `class="ct-point"`); n != 31 {
		t.Errorf("graph points = %d", n)
	}
}

func TestFailures(t *testing.T) {
	a, fake := setup(t)
	fake.failNext.Store(1)
	status, h, body := get(t, a, "/api?username=FuSan21")
	if status != 502 || h.Get("Cache-Control") != "no-store" || !strings.Contains(body, "Could not load data from GitHub") {
		t.Errorf("outage: %d %q %s", status, h.Get("Cache-Control"), body)
	}

	fake.failNext.Store(1)
	status, _, body = get(t, a, "/streak?user=FuSan21&theme=gruvbox")
	if status != 502 || !strings.Contains(body, `fill="#282828"`) {
		t.Errorf("streak error card: %d", status)
	}
	if err := render.CheckWellFormed(body); err != nil {
		t.Error(err)
	}

	if status, _, _ := get(t, a, "/streak?user=FuSan21&timezone=Not/AZone"); status != 400 {
		t.Errorf("invalid timezone: %d", status)
	}
}

var viewsRe = regexp.MustCompile(`<title>Profile views: ([\d,]+)</title>`)

func TestViews(t *testing.T) {
	a, _ := setup(t)
	_, h, body := get(t, a, "/views?username=FuSan21")
	if !strings.Contains(h.Get("Cache-Control"), "no-store") {
		t.Errorf("views cache-control: %q", h.Get("Cache-Control"))
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(viewsRe.FindStringSubmatch(body)[1], ",", ""))
	if n != 1719 {
		t.Errorf("first view = %d", n)
	}
	_, _, body = get(t, a, "/views?username=FuSan21&label=Visitors&color=ff69b4")
	for _, want := range []string{"1,720", "Visitors", "#ff69b4"} {
		if !strings.Contains(body, want) {
			t.Errorf("badge missing %q", want)
		}
	}

	file := filepath.Join(t.TempDir(), "v.json")
	v := cards.LoadViews(file, 5)
	v.Count()
	if err := v.Flush(); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(file); string(raw) != `{"views":6}` {
		t.Errorf("saved %s", raw)
	}
	if again := cards.LoadViews(file, 100); again.Count() != 7 {
		t.Error("restart did not continue from the saved count")
	}
}

func TestCache(t *testing.T) {
	c := gh.NewCache()
	n := 0
	load := func() (int, error) {
		n++
		if n == 2 {
			return 0, errors.New("boom")
		}
		return n, nil
	}
	for i, want := range []int{1, 1, 3} { // the failed refresh serves the stale value
		if got, err := gh.Cached(c, "k", 0, load); got != want || err != nil {
			t.Errorf("call %d: %d, %v", i, got, err)
		}
	}

	// Concurrent callers share one load.
	var loads atomic.Int32
	release := make(chan struct{})
	slow := func() (int, error) { loads.Add(1); <-release; return 42, nil }
	var wg sync.WaitGroup
	results := make([]int, 10)
	for i := range results {
		wg.Go(func() { results[i], _ = gh.Cached(c, "shared", time.Hour, slow) })
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if loads.Load() != 1 || fmt.Sprint(results) != fmt.Sprint([]int{42, 42, 42, 42, 42, 42, 42, 42, 42, 42}) {
		t.Errorf("loads=%d results=%v", loads.Load(), results)
	}

	// A panicking load is an error, and doesn't leave waiters hanging.
	if _, err := gh.Cached(c, "p", 0, func() (int, error) { panic("bad") }); err == nil {
		t.Error("panic not turned into an error")
	}
}

func TestGitHubBudget(t *testing.T) {
	a, fake := setup(t)
	a.GitHub.Budget = gh.NewBucket(2, time.Hour)
	// The stats card needs two calls (two pages of repos); the next new cache key finds the budget empty.
	if status, _, _ := get(t, a, "/api?username=FuSan21"); status != 200 {
		t.Fatalf("first card: %d", status)
	}
	status, h, body := get(t, a, "/api?username=FuSan21&commits_year=2025")
	if status != 503 || h.Get("Cache-Control") != "no-store" || !strings.Contains(body, "Too many card requests") {
		t.Errorf("over budget: %d %q", status, h.Get("Cache-Control"))
	}
	if fake.calls.Load() != 2 {
		t.Errorf("GitHub calls = %d, want 2", fake.calls.Load())
	}
	// Cached data is still served without spending budget.
	if status, _, _ := get(t, a, "/api?username=FuSan21&theme=dark"); status != 200 {
		t.Errorf("cached card: %d", status)
	}
}

func TestYearParamsAreClamped(t *testing.T) {
	a, fake := setup(t)
	get(t, a, "/api?username=FuSan21")
	calls := fake.calls.Load()
	for _, y := range []string{"1999", "3000", "abc", "-5"} {
		_, _, body := get(t, a, "/api?username=FuSan21&commits_year="+y)
		if !strings.Contains(body, "Total Commits (last year)") {
			t.Errorf("commits_year=%s not dropped", y)
		}
	}
	if fake.calls.Load() != calls {
		t.Errorf("invalid years made %d GitHub calls", fake.calls.Load()-calls)
	}
	if _, _, body := get(t, a, "/api?username=FuSan21&commits_year=2025"); !strings.Contains(body, "Total Commits (2025)") {
		t.Error("a valid commits_year was dropped")
	}
	if _, h, _ := get(t, a, "/oss?username=FuSan21"); h.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff header missing")
	}
}
