package cards

import (
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/FuSan21/github-stats/cards/internal/render"
)

func params(t *testing.T, q string) Params {
	t.Helper()
	v, err := url.ParseQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	return NewParams(v)
}

func wellFormed(t *testing.T, svg string) string {
	t.Helper()
	if err := CheckWellFormed(svg); err != nil {
		t.Fatalf("%v\n%s", err, svg)
	}
	return svg
}

func contains(t *testing.T, svg string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(svg, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func lacks(t *testing.T, svg string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(svg, u) {
			t.Errorf("unexpected %q", u)
		}
	}
}

func TestHelpers(t *testing.T) {
	for _, c := range []struct {
		n, precision int
		want         string
	}{{999, -1, "999"}, {1234, -1, "1.2k"}, {1000, -1, "1k"}, {1234, 2, "1.23k"}} {
		if got := KFormat(c.n, c.precision); got != c.want {
			t.Errorf("KFormat(%d, %d) = %q, want %q", c.n, c.precision, got, c.want)
		}
	}
	if got := MeasureText("Most Used Languages", 10); math.Abs(got-99.92) > 0.01 {
		t.Errorf("MeasureText = %v", got)
	}
	if got := Thousands(1234567); got != "1,234,567" {
		t.Errorf("Thousands = %q", got)
	}
	if got := Truncate("héllo world", 5); got != "héll…" {
		t.Errorf("Truncate = %q", got)
	}
}

func TestRank(t *testing.T) {
	if r := CalculateRank(RankInput{}); r.Level != "C" || r.Percentile != 100 {
		t.Errorf("zero profile: %+v", r)
	}
	if r := CalculateRank(RankInput{Commits: 1e6, PRs: 1e6, Issues: 1e6, Reviews: 1e6, Stars: 1e9, Followers: 1e9}); r.Level != "S" {
		t.Errorf("huge profile: %+v", r)
	}
	// Worked by hand from the readme-stats formula: weighted CDF sum 10.4975 / 12.
	r := CalculateRank(RankInput{Commits: 812, PRs: 120, Issues: 34, Reviews: 14, Stars: 1535, Followers: 40})
	if r.Level != "A" || math.Abs(r.Percentile-12.5207) > 0.001 {
		t.Errorf("typical profile: %+v", r)
	}
}

var testStats = Stats{
	Name: "Fu San", TotalStars: 1535, TotalCommits: 812, TotalPRs: 120, TotalPRsMerged: 100,
	TotalReviews: 14, TotalIssues: 34, TotalDiscussionsStarted: 2, TotalDiscussionsAnswered: 1,
	ContributedTo: 23, Followers: 40, RepoStars: map[string]int{"big": 1500, "small": 30, "tiny": 5},
}

func TestStatsCard(t *testing.T) {
	svg := wellFormed(t, RenderStats(testStats, params(t, "theme=gruvbox&show_icons=true"), -1))
	contains(t, svg, "Fu San&#39;s GitHub Stats", "Total Commits (last year):", ">1.5k<", "fill: #fabd2f", `width="467"`)
	lacks(t, svg, "Total PRs Reviewed")

	svg = wellFormed(t, RenderStats(testStats, params(t, "show=reviews,prs_merged_percentage&hide=issues&exclude_repo=big&number_format=long"), -1))
	contains(t, svg, "Total PRs Reviewed:", "83.33 %", ">35<")
	lacks(t, svg, "Total Issues")

	contains(t, RenderStats(testStats, params(t, ""), 4321), "Total Commits:", ">4.3k<")
	rankOnly := wellFormed(t, RenderStats(testStats, params(t, "hide=stars,commits,prs,issues,contribs"), -1))
	contains(t, rankOnly, "Fu San&#39;s GitHub Rank", `width="290"`)

	for _, icon := range []string{"github", "percentile", "default"} {
		wellFormed(t, RenderStats(testStats, params(t, "rank_icon="+icon), -1))
	}
}

func TestStatsCardEscapesAndValidates(t *testing.T) {
	svg := wellFormed(t, RenderStats(testStats, params(t, "custom_title="+url.QueryEscape("<b>&")+"&title_color="+url.QueryEscape("red;}x")), -1))
	contains(t, svg, "&lt;b&gt;&amp;")
	lacks(t, svg, "red;}x")
	contains(t, wellFormed(t, RenderStats(testStats, params(t, "bg_color=45,ff0000,00ff00"), -1)), `fill="url(#gradient)"`, "<linearGradient")
}

var testRepos = []RepoLanguages{
	{"a", []RepoLang{{"TypeScript", "#3178c6", 600}, {"CSS", "#663399", 100}}},
	{"b", []RepoLang{{"Kotlin", "", 300}, {"TypeScript", "#3178c6", 50}}},
}

func TestTopLanguages(t *testing.T) {
	got := TopLanguages(testRepos, nil, 1, 0)
	want := []Lang{{"TypeScript", "#3178c6", 650}, {"Kotlin", "#858585", 300}, {"CSS", "#663399", 100}}
	if !slices.Equal(got, want) {
		t.Errorf("TopLanguages = %v", got)
	}
	if got := TopLanguages(testRepos, []string{"a"}, 1, 0); got[0].Name != "Kotlin" || len(got) != 2 {
		t.Errorf("exclude_repo: %v", got)
	}
	// count_weight=1 multiplies by the number of repos using the language.
	if got := TopLanguages(testRepos, nil, 0, 1); got[0].Name != "TypeScript" || got[0].Size != 2 {
		t.Errorf("count weight: %v", got)
	}
}

func TestTopLanguagesCard(t *testing.T) {
	langs := TopLanguages(testRepos, nil, 1, 0)
	for _, layout := range layouts {
		t.Run(layout, func(t *testing.T) {
			contains(t, wellFormed(t, RenderTopLangs(langs, params(t, "layout="+layout+"&theme=gruvbox"))), "Most Used Languages", "Kotlin")
		})
	}
	svg := wellFormed(t, RenderTopLangs(langs, params(t, "layout=compact&hide=css")))
	contains(t, svg, "TypeScript 68.42%")
	lacks(t, svg, "CSS ")
	contains(t, wellFormed(t, RenderTopLangs(langs[:1], params(t, "layout=donut"))), "<circle")
	contains(t, wellFormed(t, RenderTopLangs(langs, params(t, "stats_format=bytes&hide_progress=true"))), "650.0 B")
	contains(t, wellFormed(t, RenderTopLangs(nil, params(t, ""))), "No languages data.")
}

// Wed 2026-10-21 .. Sat 2026-10-31
var testCalendar = []Day{
	{"2026-10-21", 3}, {"2026-10-22", 1}, {"2026-10-23", 4},
	{"2026-10-24", 0}, // Saturday
	{"2026-10-25", 2}, {"2026-10-26", 0}, {"2026-10-27", 1}, {"2026-10-28", 1},
	{"2026-10-29", 0}, {"2026-10-30", 5},
	{"2026-10-31", 0}, // today, not yet contributed
}

func TestDailyStreak(t *testing.T) {
	s, err := DailyStats(testCalendar, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.TotalContributions != 17 || s.FirstContribution != "2026-10-21" {
		t.Errorf("totals: %+v", s)
	}
	if want := (Streak{"2026-10-21", "2026-10-23", 3}); s.Longest != want {
		t.Errorf("longest = %+v", s.Longest)
	}
	// An empty today doesn't break the streak.
	if want := (Streak{"2026-10-30", "2026-10-30", 1}); s.Current != want {
		t.Errorf("current = %+v", s.Current)
	}
}

func TestExcludedDaysBridgeAStreak(t *testing.T) {
	if got := NormalizeDays([]string{"saturday", " MON", "xyz"}); !slices.Equal(got, []string{"Sat", "Mon"}) {
		t.Errorf("NormalizeDays = %v", got)
	}
	s, _ := DailyStats(testCalendar, []string{"Sat", "Mon"})
	if want := (Streak{"2026-10-21", "2026-10-28", 8}); s.Longest != want {
		t.Errorf("longest = %+v", s.Longest)
	}
}

func TestWeeklyStreak(t *testing.T) {
	s, _ := WeeklyStats(testCalendar)
	if want := (Streak{"2026-10-18", "2026-10-25", 2}); s.Longest != want {
		t.Errorf("longest = %+v", s.Longest)
	}
	if s.Current.Length != 2 || s.FirstContribution != "2026-10-21" {
		t.Errorf("current/first: %+v", s)
	}
}

func TestUntilToday(t *testing.T) {
	if got := UntilToday([]Day{{"2026-10-31", 1}, {"2026-11-01", 0}}, "2026-10-31"); len(got) != 1 {
		t.Errorf("empty tomorrow kept: %v", got)
	}
	if got := UntilToday([]Day{{"2026-10-31", 1}, {"2026-11-01", 2}}, "2026-10-31"); len(got) != 2 {
		t.Errorf("active tomorrow dropped: %v", got)
	}
	if _, err := Today("Not/AZone", time.Now()); err == nil {
		t.Error("invalid timezone accepted")
	}
	if got, _ := Today("Asia/Dhaka", time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)); got != "2026-11-01" {
		t.Errorf("Today in Dhaka = %s", got)
	}
}

func TestFormatDate(t *testing.T) {
	for _, c := range []struct{ date, format, want string }{
		{"2026-03-05", "", "Mar 5"},
		{"2024-03-05", "", "Mar 5, 2024"},
		{"2024-03-05", "j/n[/Y]", "5/3/2024"},
		{"2026-03-05", "j/n[/Y]", "5/3"},
		{"2026-03-05", "D, d.m", "Thu, 05.03"},
		{"2026-03-05", `l \t\h\e jS`, "Thursday the 5S"},
	} {
		if got := FormatDate(c.date, c.format, 2026); got != c.want {
			t.Errorf("FormatDate(%s, %q) = %q, want %q", c.date, c.format, got, c.want)
		}
	}
}

func TestStreakCard(t *testing.T) {
	s, _ := DailyStats(testCalendar, nil)
	svg := wellFormed(t, RenderStreak(s, params(t, "theme=gruvbox&hide_border=true&ring=ff0000"), 2026))
	contains(t, svg, "Current Streak", "Oct 21 - Oct 23", `stroke="#0000"`, `stroke="#ff0000"`)
	two := wellFormed(t, RenderStreak(s, params(t, "hide_total_contributions=true"), 2026))
	lacks(t, two, "Total Contributions")
	excluded, _ := DailyStats(testCalendar, []string{"Sat"})
	contains(t, wellFormed(t, RenderStreak(excluded, params(t, "theme=dark&background=45,ff0000,0000ff"), 2026)), "* Excluding Sat", "url(#gradient)")
	contains(t, wellFormed(t, StreakError("No contributions found.", params(t, "theme=gruvbox"))), `fill="#282828"`)
}

func TestYTicks(t *testing.T) {
	for _, c := range []struct {
		max  int
		want []int
	}{
		{0, []int{0, 1}},
		{7, []int{0, 1, 2, 3, 4, 5, 6, 7}},
		{40, []int{0, 5, 10, 15, 20, 25, 30, 35, 40}},
	} {
		if got := YTicks(c.max, 270, 20); !slices.Equal(got, c.want) {
			t.Errorf("YTicks(%d) = %v", c.max, got)
		}
	}
	if got := YTicks(41, 270, 20); got[len(got)-1] != 45 {
		t.Errorf("YTicks(41) top = %d", got[len(got)-1])
	}
}

func TestMonotonePathNeverDipsBelowItsPoints(t *testing.T) {
	var pts []point
	for i, v := range []float64{0, 10, 0, 0, 8} {
		pts = append(pts, point{float64(i * 10), 100 - v})
	}
	for _, m := range regexp.MustCompile(`,(-?[\d.]+)`).FindAllStringSubmatch(MonotonePath(pts), -1) {
		if y, _ := strconv.ParseFloat(m[1], 64); y > 100 {
			t.Errorf("curve dips to y=%v", y)
		}
	}
}

func TestGraphRange(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if r := GraphRangeFrom(params(t, "days=7"), now); r.Custom || r.Days != 7 {
		t.Errorf("days=7: %+v", r)
	}
	if r := GraphRangeFrom(params(t, "days=500"), now); r.Days != 31 {
		t.Errorf("days=500: %+v", r)
	}
	if r := GraphRangeFrom(params(t, "from=2026-10-01&to=2026-10-11"), now); !r.Custom || r.Days != 10 {
		t.Errorf("from/to: %+v", r)
	}
	if r := GraphRangeFrom(params(t, "from=2026-10-11&to=2026-10-01"), now); r.Custom {
		t.Errorf("reversed from/to accepted: %+v", r)
	}
}

func TestGraphCard(t *testing.T) {
	var days []Day
	for i := range 31 {
		days = append(days, Day{"2026-10-" + strconv.Itoa(100 + i + 1)[1:], i % 4})
	}
	svg := wellFormed(t, RenderGraph(GraphData{"Fu San", days}, params(t, "theme=gruvbox&area=true&hide_border=true"), "FuSan21"))
	contains(t, svg, "Fu San&#39;s Contribution Graph", `class="ct-area"`, `stroke="#0000"`)
	if n := strings.Count(svg, `class="ct-point"`); n != 31 {
		t.Errorf("points = %d", n)
	}
	untitled := wellFormed(t, RenderGraph(GraphData{"", days[:1]}, params(t, "hide_title=true&grid=false"), "FuSan21"))
	contains(t, untitled, "<title>FuSan21&#39;s contribution graph</title>")
	lacks(t, untitled, `class="ct-grid"`, `class="header"`)
}

func ossFixture() []PR {
	var prs []PR
	for i, state := range []string{"MERGED", "OPEN", "MERGED", "MERGED", "OPEN"} {
		pr := PR{Title: "A fairly long pull request title number " + strconv.Itoa(i) + " that needs truncating somewhere", State: state}
		pr.Repository = &struct {
			NameWithOwner    string  `json:"nameWithOwner"`
			IsPrivate        bool    `json:"isPrivate"`
			StargazerCount   int     `json:"stargazerCount"`
			ViewerPermission *string `json:"viewerPermission"`
			Owner            struct {
				Login string `json:"login"`
			} `json:"owner"`
		}{NameWithOwner: "some-org/project-" + strconv.Itoa(i), StargazerCount: 1000 * i}
		prs = append(prs, pr)
	}
	return prs
}

func TestOSSCardWide(t *testing.T) {
	narrow := wellFormed(t, RenderOSS(ossFixture(), params(t, "theme=gruvbox"), "FuSan21"))
	contains(t, narrow, `width="495"`, `translate(25, 124)`) // one column: row 2 below row 1
	wide := wellFormed(t, RenderOSS(ossFixture(), params(t, "theme=gruvbox&card_width=990"), "FuSan21"))
	// Two 455 px columns: PR 2 sits right of PR 1, PR 3 starts the second row.
	contains(t, wide, `width="990"`, `translate(25, 78)`, `translate(510, 78)`, `translate(25, 124)`, `x="455" y="1" text-anchor="end"`)
	if strings.Count(wide, "translate(") > strings.Count(narrow, "translate(") {
		t.Error("wide layout drew extra rows")
	}
	if h := regexp.MustCompile(`height="(\d+)"`).FindStringSubmatch(wide)[1]; h != "226" { // 3 rows instead of 5
		t.Errorf("wide height = %s, want 226", h)
	}
	contains(t, wellFormed(t, RenderOSS(ossFixture(), params(t, "card_width=5000"), "FuSan21")), `width="1200"`)
	contains(t, wellFormed(t, RenderOSS(ossFixture(), params(t, "card_width=10"), "FuSan21")), `width="495"`)
}

// ossPRs builds PRs newest first from "state repo" pairs.
func ossPRs(specs ...string) []PR {
	var prs []PR
	for i, s := range specs {
		state, repo, _ := strings.Cut(s, " ")
		pr := ossFixture()[0]
		pr.Title, pr.State = "PR "+strconv.Itoa(i), state
		r := *pr.Repository
		r.NameWithOwner = repo
		pr.Repository = &r
		prs = append(prs, pr)
	}
	return prs
}

func TestOSSGroupsByRepo(t *testing.T) {
	prs := ossPRs("OPEN a/x", "MERGED b/y", "MERGED a/x", "MERGED a/x", "OPEN a/x", "CLOSED a/x", "MERGED c/z")
	svg := wellFormed(t, RenderOSS(prs, params(t, ""), "FuSan21"))
	// a/x shows its newest PR (PR 0, open), with the rest summarised; the closed one isn't counted.
	contains(t, svg, ">PR 0<", ">+2 merged, +1 open<", ">PR 1<", ">PR 6<")
	lacks(t, svg, ">PR 2<", ">PR 3<", ">PR 4<", ">PR 5<")
	if n := strings.Count(svg, `class="repo"`); n != 3 {
		t.Errorf("entries = %d, want 3 (one per repo)", n)
	}
	// Rows: a/x is 46+18 tall, so b/y starts at 78+64.
	contains(t, svg, `translate(25, 78)`, `translate(25, 142)`, `translate(25, 188)`)
	contains(t, svg, "4 merged PRs across 3 projects")

	// limit counts repos, not PRs.
	if n := strings.Count(RenderOSS(prs, params(t, "limit=2"), "FuSan21"), `class="repo"`); n != 2 {
		t.Errorf("limit=2 showed %d entries", n)
	}

	// show_open=false: a/x's newest shown PR is PR 2, and open ones aren't counted.
	merged := wellFormed(t, RenderOSS(prs, params(t, "show_open=false"), "FuSan21"))
	contains(t, merged, ">PR 2<", ">+1 merged<")
	lacks(t, merged, "open<", ">PR 0<")

	// Two columns: a grid row is as tall as its tallest entry.
	wide := wellFormed(t, RenderOSS(prs, params(t, "card_width=990"), "FuSan21"))
	contains(t, wide, `translate(25, 78)`, `translate(510, 78)`, `translate(25, 142)`)
}
