package cards

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/FuSan21/github-stats/cards/internal/gh"
	. "github.com/FuSan21/github-stats/cards/internal/render"
)

// PR is a pull request to someone else's public repository.
type PR struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	State      string `json:"state"` // OPEN, CLOSED or MERGED
	Repository *struct {
		NameWithOwner    string  `json:"nameWithOwner"`
		IsPrivate        bool    `json:"isPrivate"`
		StargazerCount   int     `json:"stargazerCount"`
		ViewerPermission *string `json:"viewerPermission"`
		Owner            struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

const ossQuery = `
query ($q: String!) {
  search(query: $q, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
        title url state createdAt mergedAt
        repository { nameWithOwner isPrivate stargazerCount viewerPermission owner { login } }
      }
    }
  }
}`

// FetchOSSPRs loads login's 100 most recent PRs to public repositories they can't push to.
func FetchOSSPRs(ctx context.Context, c *gh.Client, cache *gh.Cache, login string) ([]PR, error) {
	return gh.Cached(cache, "oss:"+login, gh.DataTTL, func() ([]PR, error) {
		var data struct {
			Search struct {
				Nodes []PR `json:"nodes"`
			} `json:"search"`
		}
		q := fmt.Sprintf("is:pr author:%s is:public -user:%[1]s sort:created-desc", login)
		if err := c.GraphQL(ctx, ossQuery, map[string]any{"q": q}, &data); err != nil {
			return nil, err
		}
		var prs []PR
		for _, pr := range data.Search.Nodes {
			r := pr.Repository
			if r == nil || r.IsPrivate || strings.EqualFold(r.Owner.Login, login) {
				continue
			}
			// Push access means an employer or client repo, not a third-party project.
			if r.ViewerPermission != nil && slices.Contains([]string{"ADMIN", "MAINTAIN", "WRITE"}, *r.ViewerPermission) {
				continue
			}
			prs = append(prs, pr)
		}
		return prs, nil
	})
}

// ossWidth is the default card width, matching the readme-stats cards.
const ossWidth = 495

type ossTheme struct{ bg, border, title, text, muted, repo, merged, open string }

var ossThemes = map[string]ossTheme{
	"gruvbox": {"#282828", "#3c3836", "#fabd2f", "#ebdbb2", "#a89984", "#8ec07c", "#d3869b", "#b8bb26"},
	"dark":    {"#0d1117", "#30363d", "#58a6ff", "#e6edf3", "#8b949e", "#3fb950", "#a371f7", "#3fb950"},
	"default": {"#fffefe", "#e4e2e2", "#2f80ed", "#434d58", "#6a737d", "#1a7f37", "#8250df", "#1a7f37"},
}

func plural(n int, word string) string {
	if n == 1 {
		return Thousands(n) + " " + word
	}
	return Thousands(n) + " " + word + "s"
}

// RenderOSS draws the open source contributions card for user.
func RenderOSS(prs []PR, p Params, user string) string {
	t, ok := ossThemes[p.Str("theme")]
	if !ok {
		t = ossThemes["gruvbox"]
	}
	limit := p.IntOr("limit", 0)
	if limit == 0 {
		limit = 6
	}
	limit = Clamp(limit, 1, 10)
	showOpen := p.Bool("show_open", true)
	borderOpacity := 1
	if p.Bool("hide_border", false) {
		borderOpacity = 0
	}

	// One entry per repository, for its newest shown PR; the repository's other
	// shown PRs are summarised on a third line ("+2 merged, +1 open").
	type entry struct {
		pr                   PR
		moreMerged, moreOpen int
	}
	var shown []*entry
	byRepo := map[string]*entry{}
	projects := map[string]bool{}
	merged := 0
	for _, pr := range prs {
		repo := pr.Repository.NameWithOwner
		if pr.State == "MERGED" {
			merged++
			projects[repo] = true
		}
		if pr.State != "MERGED" && !(showOpen && pr.State == "OPEN") {
			continue
		}
		if e := byRepo[repo]; e != nil {
			if pr.State == "MERGED" {
				e.moreMerged++
			} else {
				e.moreOpen++
			}
			continue
		}
		e := &entry{pr: pr}
		byRepo[repo] = e
		if len(shown) < limit {
			shown = append(shown, e)
		}
	}

	// card_width >= 900 lays the entries out in two columns, newest first, left
	// to right. That suits a README row of its own, where GitHub scales the card
	// down to the column width, like the activity graph.
	W := Clamp(p.IntOr("card_width", ossWidth), ossWidth, 1200)
	cols := 1
	if W >= 900 {
		cols = 2
	}
	const rowH, moreH, top, pad, gap = 46, 18, 78, 25, 30
	colW := (W - 2*pad - gap*(cols-1)) / cols
	// Truncation lengths fit the default 445 px column; scale them with the column.
	repoMax, titleMax := 42*colW/445, 58*colW/445
	more := func(e *entry) string {
		var parts []string
		if e.moreMerged > 0 {
			parts = append(parts, fmt.Sprintf("+%d merged", e.moreMerged))
		}
		if e.moreOpen > 0 {
			parts = append(parts, fmt.Sprintf("+%d open", e.moreOpen))
		}
		return strings.Join(parts, ", ")
	}
	var rows strings.Builder
	y := top
	for i, e := range shown {
		pr := e.pr
		color, label := t.open, "Open"
		if pr.State == "MERGED" {
			color, label = t.merged, "Merged"
		}
		stars := ""
		if n := pr.Repository.StargazerCount; n > 0 {
			stars = fmt.Sprintf(`<text x="%d" y="1" text-anchor="end" class="muted">★ %s</text>`, colW, Thousands(n))
		}
		extra := ""
		if m := more(e); m != "" {
			extra = fmt.Sprintf("\n  <text x=\"62\" y=\"38\" class=\"more\">%s</text>", m)
		}
		if i > 0 {
			rows.WriteString("\n")
		}
		fmt.Fprintf(&rows, `<g transform="translate(%d, %d)">
  <rect x="0" y="-11" width="52" height="16" rx="8" fill="none" stroke="%s"/>
  <text x="26" y="1" text-anchor="middle" class="pill" fill="%[3]s">%s</text>
  <text x="62" y="1" class="repo">%s</text>
  %s
  <text x="62" y="20" class="pr">%s</text>%s
</g>`, pad+(i%cols)*(colW+gap), y, color, label, Esc(Truncate(pr.Repository.NameWithOwner, repoMax)), stars, Esc(Truncate(pr.Title, titleMax)), extra)
		// A grid row is as tall as its tallest entry, so the columns stay aligned.
		if i%cols == cols-1 || i == len(shown)-1 {
			h := rowH
			for _, other := range shown[i-i%cols : i+1] {
				if more(other) != "" {
					h = rowH + moreH
				}
			}
			y += h
		}
	}
	H := max(y, top+rowH) + 10
	if len(shown) == 0 {
		fmt.Fprintf(&rows, `<text x="25" y="%d" class="muted">No public contributions yet.</text>`, top)
	}

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%[1]d" height="%[2]d" viewBox="0 0 %[1]d %[2]d" role="img" aria-label="%[3]s's open source contributions">
<title>%[3]s's open source contributions</title>
<style>
  .title { font: 600 18px 'Segoe UI', Ubuntu, sans-serif; fill: %[4]s; }
  .sub { font: 400 12px 'Segoe UI', Ubuntu, sans-serif; fill: %[5]s; }
  .repo { font: 600 13px 'Segoe UI', Ubuntu, sans-serif; fill: %[6]s; }
  .pr { font: 400 12px 'Segoe UI', Ubuntu, sans-serif; fill: %[7]s; }
  .muted { font: 400 12px 'Segoe UI', Ubuntu, sans-serif; fill: %[5]s; }
  .pill { font: 600 10px 'Segoe UI', Ubuntu, sans-serif; }
  .more { font: 400 11px 'Segoe UI', Ubuntu, sans-serif; fill: %[5]s; }
</style>
<rect x="0.5" y="0.5" width="%[8]d" height="%[9]d" rx="4.5" fill="%[10]s" stroke="%[11]s" stroke-opacity="%[12]d"/>
<text x="25" y="35" class="title">Open Source Contributions</text>
<text x="25" y="55" class="sub">%[13]s across %[14]s</text>
%[15]s
</svg>`, W, H, Esc(user), t.title, t.muted, t.repo, t.text, W-1, H-1, t.bg, t.border, borderOpacity,
		plural(merged, "merged PR"), plural(len(projects), "project"), rows.String())
}
