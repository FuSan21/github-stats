// Profile cards for one GitHub user, rendered with that user's own token:
//
//	/api            stats card              /graph  contribution graph
//	/api/top-langs  top languages           /oss    PRs to other people's projects
//	/streak         contribution streak     /views  profile view counter
//
// Dokploy's Traefik routes the domain straight here; cards carry their own
// Cache-Control, and GitHub's image proxy caches them for an hour.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // zone data for TZ and ?timezone=, so the image needs no tzdata

	"github.com/FuSan21/github-stats/cards/internal/app"
	"github.com/FuSan21/github-stats/cards/internal/cards"
	"github.com/FuSan21/github-stats/cards/internal/gh"
)

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	user, token := os.Getenv("GITHUB_USER"), os.Getenv("GITHUB_TOKEN")
	if user == "" || token == "" {
		log.Fatal("GITHUB_USER and GITHUB_TOKEN are required")
	}
	initial, _ := strconv.Atoi(env("INITIAL_VIEWS", "0"))
	views := cards.LoadViews(env("VIEWS_FILE", "/data/views.json"), initial)
	flush := func() {
		if err := views.Flush(); err != nil {
			log.Printf("views flush failed: %v", err)
		}
	}

	a := &app.App{User: user, GitHub: gh.New(token), Cache: gh.NewCache(), Views: views, Now: time.Now}
	srv := &http.Server{
		Addr:              ":" + env("PORT", "7000"),
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		for t := time.NewTicker(5 * time.Second); ; {
			select {
			case <-t.C:
				flush()
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
	}()
	drained := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		close(drained)
	}()

	log.Printf("cards listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-drained // in-flight requests (and their view counts) finish first
	flush()
}
