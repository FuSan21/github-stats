package cards

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/FuSan21/github-stats/cards/internal/gh"
	. "github.com/FuSan21/github-stats/cards/internal/render"
)

const graphQuery = `
query ($login: String!, $from: DateTime!, $to: DateTime!) {
  user(login: $login) {
    name
    contributionsCollection(from: $from, to: $to) {
      contributionCalendar { weeks { contributionDays { contributionCount date } } }
    }
  }
}`

// GraphRange is the window to plot: the last Days days, or an explicit from/to in the past.
type GraphRange struct {
	Custom   bool
	From, To time.Time
	Days     int
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func GraphRangeFrom(p Params, now time.Time) GraphRange {
	from, to := p.Str("from"), p.Str("to")
	if dateRe.MatchString(from) && dateRe.MatchString(to) {
		f, ferr := time.Parse(time.DateOnly, from)
		t, terr := time.Parse(time.DateOnly, to)
		if ferr == nil && terr == nil && f.Before(t) && !t.After(now) {
			return GraphRange{true, f, t, int(math.Round(t.Sub(f).Hours() / 24))}
		}
	}
	days := p.IntOr("days", 0)
	if days <= 0 || days > 90 {
		days = 31
	}
	// One day ahead in case this server is behind GitHub's clock.
	return GraphRange{false, now.AddDate(0, 0, -days), now.AddDate(0, 0, 1), days}
}

type GraphData struct {
	Name string // "" when the profile has no name
	Days []Day
}

func FetchGraph(ctx context.Context, c *gh.Client, cache *gh.Cache, login string, r GraphRange) (GraphData, error) {
	// Hour resolution keeps the cache key stable for the TTL.
	key := fmt.Sprintf("graph:%s:%d:%s", login, r.Days, r.To.UTC().Format("2006-01-02T15"))
	if r.Custom {
		key = fmt.Sprintf("graph:%s:%s:%s", login, r.From.Format(time.DateOnly), r.To.Format(time.DateOnly))
	}
	return gh.Cached(cache, key, gh.DataTTL, func() (GraphData, error) {
		var data struct {
			User *struct {
				Name                    *string `json:"name"`
				ContributionsCollection struct {
					ContributionCalendar calendar `json:"contributionCalendar"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		}
		vars := map[string]any{"login": login, "from": r.From.UTC().Format(time.RFC3339), "to": r.To.UTC().Format(time.RFC3339)}
		if err := c.GraphQL(ctx, graphQuery, vars, &data); err != nil {
			return GraphData{}, err
		}
		if data.User == nil {
			return GraphData{}, &gh.Error{Message: "Could not find that user", Status: http.StatusNotFound}
		}
		days := data.User.ContributionsCollection.ContributionCalendar.days()
		if !r.Custom {
			// An empty last day is today not having started (or no commits yet); drop it.
			if n := len(days); n > 0 && days[n-1].Count == 0 {
				days = days[:n-1]
			}
			days = days[max(len(days)-r.Days, 0):]
		}
		name := ""
		if data.User.Name != nil {
			name = *data.User.Name
		}
		return GraphData{name, days}, nil
	})
}

// --------------------------------------------------------------- render

// graph themes: areaColor, bgColor, borderColor, color, titleColor, lineColor, pointColor (bare hex).
var graphThemes = loadThemes[map[string]string]("graph.json")

func graphColors(p Params) map[string]string {
	t, ok := graphThemes[p.Str("theme")]
	if !ok {
		t = graphThemes["default"]
	}
	pick := func(param, fallback string) string {
		if v := p.Str(param); IsHex(v) {
			return v
		}
		return fallback
	}
	border := t["borderColor"]
	if p.Bool("hide_border", false) {
		border = "0000"
	}
	color := pick("color", t["color"])
	title := t["titleColor"]
	if IsHex(p.Str("color")) {
		title = color
	}
	return map[string]string{
		"area":   pick("area_color", t["areaColor"]),
		"bg":     pick("bg_color", t["bgColor"]),
		"border": pick("border_color", border),
		"color":  color,
		"title":  pick("title_color", title),
		"line":   pick("line", t["lineColor"]),
		"point":  pick("point", t["pointColor"]),
	}
}

// YTicks are round integer ticks at least minSpace px apart, like Chartist's onlyInteger axis.
func YTicks(maxValue int, length, minSpace float64) []int {
	top := max(maxValue, 1)
	maxTicks := max(int(length/minSpace), 1)
	step := 1
	for mag := 1; ; mag *= 10 {
		found := false
		for _, m := range []int{1, 2, 5} {
			if s := m * mag; (top+s-1)/s <= maxTicks {
				step, found = s, true
				break
			}
		}
		if found {
			break
		}
	}
	var ticks []int
	for v := 0; v <= (top+step-1)/step*step; v += step {
		ticks = append(ticks, v)
	}
	return ticks
}

type point struct{ X, Y float64 }

// MonotonePath is a monotone cubic (Fritsch–Carlson) curve through pts:
// smooth, and never overshooting below zero.
func MonotonePath(pts []point) string {
	n := len(pts)
	if n == 0 {
		return ""
	}
	if n == 1 {
		return fmt.Sprintf("M%s,%s", F(pts[0].X), F(pts[0].Y))
	}
	dx := make([]float64, n-1)
	slope := make([]float64, n-1)
	for i := range n - 1 {
		dx[i] = pts[i+1].X - pts[i].X
		slope[i] = (pts[i+1].Y - pts[i].Y) / dx[i]
	}
	m := make([]float64, n)
	m[0], m[n-1] = slope[0], slope[n-2]
	for i := 1; i < n-1; i++ {
		a, b := slope[i-1], slope[i]
		if a*b > 0 {
			m[i] = 3 * (dx[i-1] + dx[i]) / ((2*dx[i]+dx[i-1])/a + (dx[i]+2*dx[i-1])/b)
		}
	}
	var d strings.Builder
	fmt.Fprintf(&d, "M%s,%s", F(pts[0].X), F(pts[0].Y))
	for i := range n - 1 {
		h := dx[i] / 3
		p0, p1 := pts[i], pts[i+1]
		fmt.Fprintf(&d, " C%s,%s %s,%s %s,%s", F(Round2(p0.X+h)), F(Round2(p0.Y+m[i]*h)), F(Round2(p1.X-h)), F(Round2(p1.Y-m[i+1]*h)), F(p1.X), F(p1.Y))
	}
	return d.String()
}

// RenderGraph draws the contribution line chart.
func RenderGraph(d GraphData, p Params, login string) string {
	c := graphColors(p)
	const width = 1200.0
	height := float64(Clamp(p.IntOr("height", 420), 200, 600))
	radius := Clamp(p.NumOr("radius", 0), 0, 16)
	grid := p.Str("grid") != "false"
	area := p.Bool("area", false)
	title := ""
	if !p.Bool("hide_title", false) {
		name := d.Name
		if name == "" {
			name = login
		}
		title = name + "'s Contribution Graph"
		if t := p.Str("custom_title"); t != "" {
			title = t
		}
	}

	// Chartist layout: padding (top 80, right 50, bottom 20, left 20) plus axis offsets (y 70, x 50).
	const x1, y1 = 90.0, 80.0
	x2, y2 := width-50, height-20-50
	maxCount := 0
	for _, day := range d.Days {
		maxCount = max(maxCount, day.Count)
	}
	ticks := YTicks(maxCount, y2-y1, 20)
	top := float64(ticks[len(ticks)-1])
	xAt := func(i int) float64 {
		if len(d.Days) > 1 {
			return x1 + (x2-x1)*float64(i)/float64(len(d.Days)-1)
		}
		return (x1 + x2) / 2
	}
	yAt := func(v float64) float64 { return y2 - (y2-y1)*v/top }
	pts := make([]point, len(d.Days))
	for i, day := range d.Days {
		pts[i] = point{Round2(xAt(i)), Round2(yAt(float64(day.Count)))}
	}
	line := MonotonePath(pts)

	var b strings.Builder
	b.WriteString("<g>")
	if grid {
		for _, t := range ticks {
			y := F(Round2(yAt(float64(t))))
			fmt.Fprintf(&b, `<line x1="%s" x2="%s" y1="%s" y2="%[3]s" class="ct-grid"/>`, F(x1), F(x2), y)
		}
		for _, pt := range pts {
			fmt.Fprintf(&b, `<line x1="%s" x2="%[1]s" y1="%s" y2="%s" class="ct-grid"/>`, F(pt.X), F(y1), F(y2))
		}
	}
	b.WriteString("</g>\n<g>")
	for _, t := range ticks {
		fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="end" class="ct-label">%d</text>`, F(x1-10), F(Round2(yAt(float64(t))+4.5)), t)
	}
	for i, day := range d.Days {
		dom, _ := strconv.Atoi(day.Date[8:])
		fmt.Fprintf(&b, `<text x="%s" y="%s" text-anchor="middle" class="ct-label">%d</text>`, F(pts[i].X), F(y2+20), dom)
	}
	b.WriteString("</g>")
	areaPath := ""
	if area && len(pts) > 0 {
		areaPath = fmt.Sprintf(`<path d="%s L%s,%s L%s,%[3]s Z" class="ct-area"/>`, line, F(pts[len(pts)-1].X), F(y2), F(pts[0].X))
	}
	var points strings.Builder
	for _, pt := range pts {
		fmt.Fprintf(&points, `<line x1="%s" y1="%s" x2="%s" y2="%[2]s" class="ct-point"/>`, F(pt.X), F(pt.Y), F(Round2(pt.X+0.01)))
	}
	header, a11y := "", title
	if title != "" {
		header = fmt.Sprintf(`<text x="%s" y="40" text-anchor="middle" class="header">%s</text>`, F(width/2), Esc(title))
	} else {
		a11y = login + "'s contribution graph"
	}

	return fmt.Sprintf(`<svg width="%[1]s" height="%[2]s" viewBox="0 0 %[1]s %[2]s" fill="none" xmlns="http://www.w3.org/2000/svg" role="img">
<title>%[3]s</title>
<style>
  svg { font: 600 18px 'Segoe UI', Ubuntu, Sans-Serif; user-select: none; }
  .header { font: 600 20px 'Segoe UI', Ubuntu, Sans-Serif; fill: #%[4]s; }
  .ct-label { font: 400 12px 'Segoe UI', Ubuntu, Sans-Serif; fill: #%[5]s; }
  .ct-axis-title { font: 400 14px 'Segoe UI', Ubuntu, Sans-Serif; fill: #%[5]s; }
  .ct-grid { stroke: #%[5]s; stroke-width: 1px; stroke-opacity: 0.3; stroke-dasharray: 2px; }
  .ct-point { stroke-width: 10px; stroke-linecap: round; stroke: #%[6]s; animation: blink 1s ease-in-out forwards; }
  .ct-line { fill: none; stroke-width: 4px; stroke-dasharray: 5000; stroke-dashoffset: 5000; stroke: #%[7]s; animation: dash 5s ease-in-out forwards; }
  .ct-area { stroke: none; fill: #%[8]s; fill-opacity: 0.1; }
  @keyframes blink { from { opacity: 0; transform: translateX(-20px); } to { opacity: 1; transform: translateX(0); } }
  @keyframes dash { to { stroke-dashoffset: 0; } }
</style>
<rect x="0.5" y="0.5" rx="%[9]s" width="%[10]s" height="%[11]s" fill="#%[12]s" stroke="#%[13]s" stroke-width="1"/>
%[14]s
%[15]s
<text x="%[16]s" y="%[17]s" text-anchor="middle" class="ct-axis-title">Days</text>
<text transform="translate(30, %[18]s) rotate(-90)" text-anchor="middle" class="ct-axis-title">Contributions</text>
%[19]s
<path d="%[20]s" class="ct-line"/>
%[21]s
</svg>`, F(width), F(height), Esc(a11y), c["title"], c["color"], c["point"], c["line"], c["area"],
		F(radius), F(width-1), F(height-1), c["bg"], c["border"], header, b.String(),
		F(x1+(x2-x1)/2), F(height-15), F(y1+(y2-y1)/2), areaPath, line, points.String())
}
