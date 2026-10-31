// Package app routes requests to cards and enforces the single user.
package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FuSan21/github-stats/cards/internal/cards"
	"github.com/FuSan21/github-stats/cards/internal/gh"
	"github.com/FuSan21/github-stats/cards/internal/render"
)

const (
	cacheOK = "public, max-age=3600"
	noStore = "max-age=0, no-cache, no-store, must-revalidate"
)

type App struct {
	User   string // the only GitHub user whose cards are served
	GitHub *gh.Client
	Cache  *gh.Cache
	Views  *cards.Views
	Now    func() time.Time
}

type route struct {
	userParam string // "username", or "user" for the streak card
	render    func(ctx context.Context, p render.Params) (string, error)
	errorCard func(msg string, p render.Params) string
}

func (a *App) routes() map[string]route {
	login := a.User // query the canonical login so the data cache is shared across spellings
	return map[string]route{
		"/api": {userParam: "username", render: func(ctx context.Context, p render.Params) (string, error) {
			stats, err := cards.FetchStats(ctx, a.GitHub, a.Cache, login, p.IntOr("commits_year", 0))
			if err != nil {
				return "", err
			}
			all := -1
			if p.Bool("include_all_commits", false) {
				if all, err = cards.FetchAllCommits(ctx, a.GitHub, a.Cache, login); err != nil {
					return "", err
				}
			}
			return cards.RenderStats(stats, p, all), nil
		}},
		"/api/top-langs": {userParam: "username", render: func(ctx context.Context, p render.Params) (string, error) {
			repos, err := cards.FetchRepoLanguages(ctx, a.GitHub, a.Cache, login)
			if err != nil {
				return "", err
			}
			langs := cards.TopLanguages(repos, p.List("exclude_repo"), p.NumOr("size_weight", 1), p.NumOr("count_weight", 0))
			return cards.RenderTopLangs(langs, p), nil
		}},
		"/streak": {userParam: "user", errorCard: cards.StreakError, render: func(ctx context.Context, p render.Params) (string, error) {
			today, err := cards.Today(p.Str("timezone"), a.Now())
			if err != nil {
				return "", err
			}
			days, err := cards.FetchCalendar(ctx, a.GitHub, a.Cache, login, p.IntOr("starting_year", 0))
			if err != nil {
				return "", err
			}
			days = cards.UntilToday(days, today)
			var s cards.StreakStats
			if p.Str("mode") == "weekly" {
				s, err = cards.WeeklyStats(days)
			} else {
				s, err = cards.DailyStats(days, cards.NormalizeDays(p.List("exclude_days")))
			}
			if err != nil {
				return "", err
			}
			year := 0
			if t, err := time.Parse(time.DateOnly, today); err == nil {
				year = t.Year()
			}
			return cards.RenderStreak(s, p, year), nil
		}},
		"/graph": {userParam: "username", render: func(ctx context.Context, p render.Params) (string, error) {
			d, err := cards.FetchGraph(ctx, a.GitHub, a.Cache, login, cards.GraphRangeFrom(p, a.Now()))
			if err != nil {
				return "", err
			}
			return cards.RenderGraph(d, p, login), nil
		}},
		"/oss": {userParam: "username", render: func(ctx context.Context, p render.Params) (string, error) {
			prs, err := cards.FetchOSSPRs(ctx, a.GitHub, a.Cache, login)
			if err != nil {
				return "", err
			}
			return cards.RenderOSS(prs, p, login), nil
		}},
	}
}

func svg(w http.ResponseWriter, status int, cache, body string) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// Handler serves every card plus /healthz.
func (a *App) Handler() http.Handler {
	routes := a.routes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		q := r.URL.Query()
		// Years outside GitHub's lifetime would only mint new data cache keys
		// (and GitHub calls); drop them so the card falls back to its default.
		for name, first := range map[string]int{"commits_year": 2008, "starting_year": 2005} {
			if y, err := strconv.Atoi(q.Get(name)); q.Has(name) && (err != nil || y < first || y > a.Now().Year()) {
				q.Del(name)
			}
		}
		p := render.NewParams(q)
		rt, ok := routes[r.URL.Path]
		isViews := r.URL.Path == "/views"
		if !ok && !isViews {
			svg(w, http.StatusNotFound, "no-store", cards.ErrorCard("Not found", cards.Colors(p)))
			return
		}
		fail := func(msg string, status int) {
			body := cards.ErrorCard(msg, cards.Colors(p))
			if rt.errorCard != nil {
				body = rt.errorCard(msg, p)
			}
			svg(w, status, "no-store", body)
		}

		// Only the token owner's cards: any other user would spend its quota, and
		// the token can see private data.
		userParam := "username"
		if ok {
			userParam = rt.userParam
		}
		if !strings.EqualFold(p.Str(userParam), a.User) || a.User == "" {
			fail("This username is not allowed", http.StatusForbidden)
			return
		}

		if isViews {
			svg(w, http.StatusOK, noStore, cards.ViewsBadge(a.Views.Count(), p))
			return
		}

		// Loads are shared through the cache, so one client going away must not
		// cancel them for the others; the HTTP client's timeout still bounds them.
		body, err := rt.render(context.WithoutCancel(r.Context()), p)
		if err != nil {
			var ghErr *gh.Error
			if errors.As(err, &ghErr) {
				fail(ghErr.Message, ghErr.Status)
				return
			}
			log.Printf("%s failed: %v", r.URL.Path, err)
			fail("Something went wrong rendering this card", http.StatusInternalServerError)
			return
		}
		svg(w, http.StatusOK, cacheOK, body)
	})
}
