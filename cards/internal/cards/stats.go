package cards

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/FuSan21/github-stats/cards/internal/gh"
	. "github.com/FuSan21/github-stats/cards/internal/render"
)

// Stats is everything the stats card shows, before per-request options.
type Stats struct {
	Name                     string
	TotalStars               int
	TotalCommits             int
	TotalPRs                 int
	TotalPRsMerged           int
	TotalReviews             int
	TotalIssues              int
	TotalDiscussionsStarted  int
	TotalDiscussionsAnswered int
	ContributedTo            int
	Followers                int
	// Stars per owned repository name, so exclude_repo can be applied per request.
	RepoStars map[string]int
}

const reposField = `
  repositories(first: 100, ownerAffiliations: OWNER, orderBy: {direction: DESC, field: STARGAZERS}, after: $after) {
    nodes { name stargazers { totalCount } }
    pageInfo { hasNextPage endCursor }
  }`

const statsQuery = `
query ($login: String!, $after: String, $startTime: DateTime) {
  user(login: $login) {
    name
    login
    commits: contributionsCollection(from: $startTime) { totalCommitContributions }
    reviews: contributionsCollection { totalPullRequestReviewContributions }
    repositoriesContributedTo(first: 1, contributionTypes: [COMMIT, ISSUE, PULL_REQUEST, REPOSITORY]) { totalCount }
    pullRequests(first: 1) { totalCount }
    mergedPullRequests: pullRequests(states: MERGED) { totalCount }
    openIssues: issues(states: OPEN) { totalCount }
    closedIssues: issues(states: CLOSED) { totalCount }
    followers { totalCount }
    repositoryDiscussions { totalCount }
    repositoryDiscussionComments(onlyAnswers: true) { totalCount }
    ` + reposField + `
  }
}`

const reposQuery = `query ($login: String!, $after: String) { user(login: $login) { ` + reposField + ` } }`

type count struct {
	TotalCount int `json:"totalCount"`
}

type repoPage struct {
	Nodes []struct {
		Name       string `json:"name"`
		Stargazers count  `json:"stargazers"`
	} `json:"nodes"`
	PageInfo pageInfo `json:"pageInfo"`
}

type pageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

// FetchStats loads the stats card data, commits counted from commitsYear (0: the last year).
func FetchStats(ctx context.Context, c *gh.Client, cache *gh.Cache, login string, commitsYear int) (Stats, error) {
	return gh.Cached(cache, fmt.Sprintf("stats:%s:%d", login, commitsYear), gh.DataTTL, func() (Stats, error) {
		var startTime any
		if commitsYear > 0 {
			startTime = fmt.Sprintf("%d-01-01T00:00:00Z", commitsYear)
		}
		var data struct {
			User struct {
				Name    *string `json:"name"`
				Login   string  `json:"login"`
				Commits struct {
					TotalCommitContributions int `json:"totalCommitContributions"`
				} `json:"commits"`
				Reviews struct {
					TotalPullRequestReviewContributions int `json:"totalPullRequestReviewContributions"`
				} `json:"reviews"`
				RepositoriesContributedTo    count    `json:"repositoriesContributedTo"`
				PullRequests                 count    `json:"pullRequests"`
				MergedPullRequests           count    `json:"mergedPullRequests"`
				OpenIssues                   count    `json:"openIssues"`
				ClosedIssues                 count    `json:"closedIssues"`
				Followers                    count    `json:"followers"`
				RepositoryDiscussions        count    `json:"repositoryDiscussions"`
				RepositoryDiscussionComments count    `json:"repositoryDiscussionComments"`
				Repositories                 repoPage `json:"repositories"`
			} `json:"user"`
		}
		if err := c.GraphQL(ctx, statsQuery, map[string]any{"login": login, "after": nil, "startTime": startTime}, &data); err != nil {
			return Stats{}, err
		}
		u := data.User

		// Repositories come sorted by stars; stop at the first page with an unstarred one.
		stars := map[string]int{}
		page := u.Repositories
		for {
			unstarred := false
			for _, r := range page.Nodes {
				stars[r.Name] = r.Stargazers.TotalCount
				unstarred = unstarred || r.Stargazers.TotalCount == 0
			}
			if !page.PageInfo.HasNextPage || unstarred {
				break
			}
			var next struct {
				User struct {
					Repositories repoPage `json:"repositories"`
				} `json:"user"`
			}
			if err := c.GraphQL(ctx, reposQuery, map[string]any{"login": login, "after": page.PageInfo.EndCursor}, &next); err != nil {
				return Stats{}, err
			}
			page = next.User.Repositories
		}

		name := u.Login
		if u.Name != nil && *u.Name != "" {
			name = *u.Name
		}
		total := 0
		for _, n := range stars {
			total += n
		}
		return Stats{
			Name:                     name,
			TotalStars:               total,
			TotalCommits:             u.Commits.TotalCommitContributions,
			TotalPRs:                 u.PullRequests.TotalCount,
			TotalPRsMerged:           u.MergedPullRequests.TotalCount,
			TotalReviews:             u.Reviews.TotalPullRequestReviewContributions,
			TotalIssues:              u.OpenIssues.TotalCount + u.ClosedIssues.TotalCount,
			TotalDiscussionsStarted:  u.RepositoryDiscussions.TotalCount,
			TotalDiscussionsAnswered: u.RepositoryDiscussionComments.TotalCount,
			ContributedTo:            u.RepositoriesContributedTo.TotalCount,
			Followers:                u.Followers.TotalCount,
			RepoStars:                stars,
		}, nil
	})
}

// FetchAllCommits is the all-time commit count from commit search, for include_all_commits.
func FetchAllCommits(ctx context.Context, c *gh.Client, cache *gh.Cache, login string) (int, error) {
	return gh.Cached(cache, "all-commits:"+login, gh.DataTTL, func() (int, error) {
		var res struct {
			TotalCount int `json:"total_count"`
		}
		err := c.REST(ctx, "/search/commits?q=author:"+url.QueryEscape(login), &res)
		return res.TotalCount, err
	})
}

// ----------------------------------------------------------------- rank

type RankInput struct {
	AllCommits                                      bool
	Commits, PRs, Issues, Reviews, Stars, Followers float64
}

type Rank struct {
	Level      string
	Percentile float64
}

// CalculateRank is readme-stats' rank: a weighted percentile against fixed medians. Lower is better.
func CalculateRank(in RankInput) Rank {
	exponentialCDF := func(x float64) float64 { return 1 - math.Pow(2, -x) }
	logNormalCDF := func(x float64) float64 { return x / (1 + x) }
	commitsMedian := 250.0
	if in.AllCommits {
		commitsMedian = 1000
	}
	parts := [][2]float64{
		{2, exponentialCDF(in.Commits / commitsMedian)},
		{3, exponentialCDF(in.PRs / 50)},
		{1, exponentialCDF(in.Issues / 25)},
		{1, exponentialCDF(in.Reviews / 2)},
		{4, logNormalCDF(in.Stars / 50)},
		{1, logNormalCDF(in.Followers / 10)},
	}
	total, sum := 0.0, 0.0
	for _, p := range parts {
		total += p[0]
		sum += p[0] * p[1]
	}
	rank := 1 - sum/total
	thresholds := []float64{1, 12.5, 25, 37.5, 50, 62.5, 75, 87.5, 100}
	levels := []string{"S", "A+", "A", "A-", "B+", "B", "B-", "C+", "C"}
	level := "C"
	for i, t := range thresholds {
		if rank*100 <= t {
			level = levels[i]
			break
		}
	}
	return Rank{level, rank * 100}
}

// --------------------------------------------------------------- render

const (
	rankCardMinWidth     = 420
	rankCardDefaultWidth = 450
	rankOnlyCardWidth    = 290
	cardMinWidth         = 287
	cardDefaultWidth     = 287
)

var optionalStats = []string{"prs_merged", "prs_merged_percentage", "reviews", "discussions_started", "discussions_answered"}

func rankIcon(kind, level string, percentile float64) string {
	switch kind {
	case "github":
		return `<svg x="-38" y="-30" height="66" width="66" aria-hidden="true" viewBox="0 0 16 16"><path d="` + githubMark + `"/></svg>`
	case "percentile":
		return `<text x="-5" y="-12" alignment-baseline="central" dominant-baseline="central" text-anchor="middle" class="rank-percentile-header">Top</text>
<text x="-5" y="12" alignment-baseline="central" dominant-baseline="central" text-anchor="middle" class="rank-percentile-text">` + ToFixed(percentile, 1) + `%</text>`
	}
	return `<text x="-5" y="3" alignment-baseline="central" dominant-baseline="central" text-anchor="middle">` + level + `</text>`
}

var endsWithS = regexp.MustCompile(`(?i)s$`)

// RenderStats draws the stats card. allCommits >= 0 replaces the last-year commit count.
func RenderStats(s Stats, p Params, allCommits int) string {
	colors := Colors(p)
	frame := FrameFrom(p)
	hide, show := p.List("hide"), p.List("show")
	showIcons := p.Bool("show_icons", false)
	hideRank := p.Bool("hide_rank", false)
	bold := p.Bool("text_bold", true)
	lineHeight := float64(p.IntOr("line_height", 25))
	longNumbers := strings.ToLower(p.Str("number_format")) == "long"
	precision := -1
	if n, ok := p.Int("number_precision"); ok {
		precision = Clamp(n, 0, 2)
	}
	commitsYear := p.IntOr("commits_year", 0)

	stars := s.TotalStars
	if exclude := p.List("exclude_repo"); len(exclude) > 0 {
		stars = 0
		for name, n := range s.RepoStars {
			if !slices.Contains(exclude, name) {
				stars += n
			}
		}
	}
	commits := s.TotalCommits
	if allCommits >= 0 {
		commits = allCommits
	}
	rank := CalculateRank(RankInput{
		AllCommits: allCommits >= 0,
		Commits:    float64(commits), PRs: float64(s.TotalPRs), Issues: float64(s.TotalIssues),
		Reviews: float64(s.TotalReviews), Stars: float64(stars), Followers: float64(s.Followers),
	})

	commitsLabel := "Total Commits"
	switch {
	case allCommits >= 0:
	case commitsYear > 0:
		commitsLabel += fmt.Sprintf(" (%d)", commitsYear)
	default:
		commitsLabel += " (last year)"
	}
	mergedPct := 0.0
	if s.TotalPRs > 0 {
		mergedPct = float64(s.TotalPRsMerged) / float64(s.TotalPRs) * 100
	}
	pctPrecision := 2
	if precision >= 0 {
		pctPrecision = precision
	}

	type item struct {
		id, icon, label string
		value           int
		text            string // preformatted value, instead of value
		unit            string
	}
	all := []item{
		{id: "stars", icon: "star", label: "Total Stars Earned", value: stars},
		{id: "commits", label: commitsLabel, value: commits},
		{id: "prs", label: "Total PRs", value: s.TotalPRs},
		{id: "prs_merged", label: "Total PRs Merged", value: s.TotalPRsMerged},
		{id: "prs_merged_percentage", label: "Merged PRs Percentage", text: ToFixed(mergedPct, pctPrecision), unit: "%"},
		{id: "reviews", label: "Total PRs Reviewed", value: s.TotalReviews},
		{id: "issues", label: "Total Issues", value: s.TotalIssues},
		{id: "discussions_started", label: "Total Discussions Started", value: s.TotalDiscussionsStarted},
		{id: "discussions_answered", label: "Total Discussions Answered", value: s.TotalDiscussionsAnswered},
		{id: "contribs", label: "Contributed to (last year)", value: s.ContributedTo},
	}
	var items []item
	for _, it := range all {
		if slices.Contains(hide, it.id) || (slices.Contains(optionalStats, it.id) && !slices.Contains(show, it.id)) {
			continue
		}
		if it.text == "" {
			it.text = strconv.Itoa(it.value)
			if !longNumbers {
				it.text = KFormat(it.value, precision)
			}
		}
		items = append(items, it)
	}

	weight := "not_bold"
	if bold {
		weight = "bold"
	}
	valueX, labelX := 120+79.01, ""
	if showIcons {
		valueX, labelX = 140+79.01, ` x="25"`
	}
	nodes := make([]string, len(items))
	desc := make([]string, len(items))
	for i, it := range items {
		icon := ""
		if showIcons {
			icon = `<svg class="icon" viewBox="0 0 16 16" version="1.1" width="16" height="16">` + icons[cmp.Or(it.icon, it.id)] + `</svg>`
		}
		unit := ""
		if it.unit != "" {
			unit = " " + it.unit
		}
		nodes[i] = fmt.Sprintf(`<g class="stagger" style="animation-delay: %dms" transform="translate(25, 0)">
  %s
  <text class="stat %[3]s"%[4]s y="12.5">%[5]s:</text>
  <text class="stat %[3]s" x="%[6]s" y="12.5">%[7]s%[8]s</text>
</g>`, (i+3)*150, icon, weight, labelX, Esc(it.label), F(valueX), it.text, unit)
		raw := it.text
		if it.unit == "" {
			raw = strconv.Itoa(it.value)
		}
		desc[i] = it.label + ": " + raw
	}

	possessive := s.Name + "'s"
	if endsWithS.MatchString(strings.TrimSpace(s.Name)) {
		possessive = s.Name + "'"
	}
	kind := "Stats"
	if len(items) == 0 {
		kind = "Rank"
	}
	title := frame.Title(possessive + " GitHub " + kind)

	height := 45 + float64(len(items)+1)*lineHeight
	if !hideRank {
		minH := 150.0
		if len(items) == 0 {
			minH = 180
		}
		height = max(height, minH)
	}

	iconWidth := 0.0
	if showIcons && len(items) > 0 {
		iconWidth = 17
	}
	var minWidth, defaultWidth float64
	switch {
	case hideRank:
		minWidth, defaultWidth = max(50+MeasureText(title, 10)*2, cardMinWidth), cardDefaultWidth
	case len(items) > 0:
		minWidth, defaultWidth = rankCardMinWidth, rankCardDefaultWidth
	default:
		minWidth, defaultWidth = rankOnlyCardWidth, rankOnlyCardWidth
	}
	minWidth += iconWidth
	defaultWidth += iconWidth
	width := max(float64(p.IntOr("card_width", int(defaultWidth))), minWidth)

	var rankX float64
	if len(items) == 0 {
		rankX = width/2 + 10
	} else {
		minX := rankCardMinWidth + iconWidth - 70
		if width > rankCardDefaultWidth {
			rankX = minX + (450-minWidth)/2 + width - rankCardDefaultWidth
		} else {
			rankX = minX + (width-minWidth)/2
		}
	}

	circumference := math.Pi * 80
	progress := Clamp(100-rank.Percentile, 0, 100)
	display := "none"
	if showIcons {
		display = "block"
	}
	css := fmt.Sprintf(`
  .stat { font: 600 14px 'Segoe UI', Ubuntu, "Helvetica Neue", Sans-Serif; fill: %[1]s; }
  @supports(-moz-appearance: auto) { .stat { font-size: 12px; } }
  .stagger { opacity: 0; animation: fadeInAnimation 0.3s ease-in-out forwards; }
  .rank-text { font: 800 24px 'Segoe UI', Ubuntu, Sans-Serif; fill: %[1]s; animation: scaleInAnimation 0.3s ease-in-out forwards; }
  .rank-percentile-header { font-size: 14px; }
  .rank-percentile-text { font-size: 16px; }
  .not_bold { font-weight: 400 }
  .bold { font-weight: 700 }
  .icon { fill: %[2]s; display: %[3]s; }
  .rank-text svg { fill: %[1]s; }
  .rank-circle-rim { stroke: %[4]s; fill: none; stroke-width: 6; opacity: 0.2; }
  .rank-circle { stroke: %[4]s; stroke-dasharray: 250; fill: none; stroke-width: 6; stroke-linecap: round; opacity: 0.8; transform-origin: -10px 8px; transform: rotate(-90deg); animation: rankAnimation 1s forwards ease-in-out; }
  @keyframes rankAnimation { from { stroke-dashoffset: %[5]s; } to { stroke-dashoffset: %[6]s; } }`,
		colors.Text, colors.Icon, display, colors.Ring, F(circumference), F((100-progress)/100*circumference))

	rankCircle := ""
	if !hideRank {
		rankCircle = fmt.Sprintf(`<g transform="translate(%s, %s)">
  <circle class="rank-circle-rim" cx="-10" cy="8" r="40"/>
  <circle class="rank-circle" cx="-10" cy="8" r="40"/>
  <g class="rank-text">%s</g>
</g>`, F(rankX), F(height/2-50), rankIcon(p.Str("rank_icon"), rank.Level, rank.Percentile))
	}

	return card{
		Frame: frame, Width: width, Height: height, Title: title, Colors: colors, CSS: css,
		A11yTitle: title + ", Rank: " + rank.Level, Desc: strings.Join(desc, ", "),
	}.render(rankCircle + `<svg x="0" y="0">` + Flex(nodes, lineHeight, true) + `</svg>`)
}
