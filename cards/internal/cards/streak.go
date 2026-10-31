package cards

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FuSan21/github-stats/cards/internal/gh"
	. "github.com/FuSan21/github-stats/cards/internal/render"
)

// Day is one day of the contribution calendar.
type Day struct {
	Date  string // YYYY-MM-DD
	Count int
}

const calendarQuery = `
query ($login: String!, $from: DateTime!, $to: DateTime!) {
  user(login: $login) {
    createdAt
    contributionsCollection(from: $from, to: $to) {
      contributionYears
      contributionCalendar { weeks { contributionDays { contributionCount date } } }
    }
  }
}`

type calendarUser struct {
	CreatedAt               string `json:"createdAt"`
	ContributionsCollection struct {
		ContributionYears    []int    `json:"contributionYears"`
		ContributionCalendar calendar `json:"contributionCalendar"`
	} `json:"contributionsCollection"`
}

type calendar struct {
	Weeks []struct {
		ContributionDays []struct {
			ContributionCount int    `json:"contributionCount"`
			Date              string `json:"date"`
		} `json:"contributionDays"`
	} `json:"weeks"`
}

func (c calendar) days() []Day {
	var out []Day
	for _, w := range c.Weeks {
		for _, d := range w.ContributionDays {
			out = append(out, Day{d.Date, d.ContributionCount})
		}
	}
	return out
}

func fetchYear(ctx context.Context, c *gh.Client, login string, year int) (calendarUser, error) {
	var data struct {
		User *calendarUser `json:"user"`
	}
	vars := map[string]any{"login": login, "from": fmt.Sprintf("%d-01-01T00:00:00Z", year), "to": fmt.Sprintf("%d-12-31T23:59:59Z", year)}
	if err := c.GraphQL(ctx, calendarQuery, vars, &data); err != nil {
		return calendarUser{}, err
	}
	if data.User == nil {
		return calendarUser{}, &gh.Error{Message: "Could not find that user", Status: http.StatusNotFound}
	}
	return *data.User, nil
}

// FetchCalendar loads every day since the account was created (or startingYear),
// fetching the years in parallel.
func FetchCalendar(ctx context.Context, c *gh.Client, cache *gh.Cache, login string, startingYear int) ([]Day, error) {
	return gh.Cached(cache, fmt.Sprintf("calendar:%s:%d", login, startingYear), gh.DataTTL, func() ([]Day, error) {
		currentYear := time.Now().Year()
		current, err := fetchYear(ctx, c, login, currentYear)
		if err != nil {
			return nil, err
		}
		created, _ := strconv.Atoi(current.CreatedAt[:min(4, len(current.CreatedAt))])
		first := created
		if startingYear > 0 {
			first = startingYear
		}
		var years []int
		for y := max(first, 2005); y < currentYear; y++ {
			years = append(years, y)
		}
		// Backdated commits (e.g. to 1970) show up as contribution years before Git existed.
		if ys := current.ContributionsCollection.ContributionYears; len(ys) > 0 && ys[len(ys)-1] < 2005 {
			years = append([]int{ys[len(ys)-1]}, years...)
		}

		results := make([]calendarUser, len(years))
		errs := make([]error, len(years))
		var wg sync.WaitGroup
		for i, y := range years {
			wg.Go(func() { results[i], errs[i] = fetchYear(ctx, c, login, y) })
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return nil, err
			}
		}

		byDate := map[string]int{}
		for _, u := range append(results, current) {
			for _, d := range u.ContributionsCollection.ContributionCalendar.days() {
				byDate[d.Date] = d.Count
			}
		}
		days := make([]Day, 0, len(byDate))
		for date, n := range byDate {
			days = append(days, Day{date, n})
		}
		sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
		return days, nil
	})
}

// ---------------------------------------------------------------- dates

var dayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

func parseDate(d string) time.Time {
	t, _ := time.Parse(time.DateOnly, d)
	return t
}

func addDays(d string, n int) string { return parseDate(d).AddDate(0, 0, n).Format(time.DateOnly) }

func previousSunday(d string) string { return addDays(d, -int(parseDate(d).Weekday())) }

// Today is today's date in the named zone ("" for the process TZ).
func Today(zone string, now time.Time) (string, error) {
	loc := time.Local
	if zone != "" {
		var err error
		if loc, err = time.LoadLocation(zone); err != nil {
			return "", &gh.Error{Message: "Invalid timezone.", Status: http.StatusBadRequest}
		}
	}
	return now.In(loc).Format(time.DateOnly), nil
}

// UntilToday keeps days up to today, plus tomorrow if it already has
// contributions (the server may be behind GitHub).
func UntilToday(days []Day, today string) []Day {
	tomorrow := addDays(today, 1)
	var out []Day
	for _, d := range days {
		if d.Date <= today || (d.Date == tomorrow && d.Count > 0) {
			out = append(out, d)
		}
	}
	return out
}

// ---------------------------------------------------------------- stats

type Streak struct {
	Start, End string
	Length     int
}

type StreakStats struct {
	Weekly             bool
	TotalContributions int
	FirstContribution  string
	Current, Longest   Streak
	ExcludedDays       []string
}

// NormalizeDays turns "monday, Tue" into ["Mon", "Tue"], dropping anything that isn't a weekday.
func NormalizeDays(days []string) []string {
	var out []string
	for _, d := range days {
		d = strings.ToLower(strings.TrimSpace(d))
		if len(d) >= 3 {
			d = strings.ToUpper(d[:1]) + d[1:3]
		}
		if slices.Contains(dayNames, d) {
			out = append(out, d)
		}
	}
	return out
}

func streaks(days []Day, skip func(string) bool) (StreakStats, error) {
	if len(days) == 0 {
		return StreakStats{}, &gh.Error{Message: "No contributions found.", Status: http.StatusNotFound}
	}
	last, first := days[len(days)-1].Date, days[0].Date
	s := StreakStats{Current: Streak{first, first, 0}, Longest: Streak{first, first, 0}}
	for _, d := range days {
		s.TotalContributions += d.Count
		if d.Count > 0 || (s.Current.Length > 0 && skip(d.Date)) {
			s.Current.Length++
			s.Current.End = d.Date
			if s.Current.Length == 1 {
				s.Current.Start = d.Date
			}
			if s.FirstContribution == "" && d.Count > 0 {
				s.FirstContribution = d.Date
			}
			if s.Current.Length > s.Longest.Length {
				s.Longest = s.Current
			}
		} else if d.Date != last {
			// The latest day (or week) doesn't break the streak yet; it isn't over.
			s.Current = Streak{last, last, 0}
		}
	}
	return s, nil
}

func DailyStats(days []Day, excluded []string) (StreakStats, error) {
	s, err := streaks(days, func(d string) bool { return slices.Contains(excluded, dayNames[parseDate(d).Weekday()]) })
	s.ExcludedDays = excluded
	return s, err
}

func WeeklyStats(days []Day) (StreakStats, error) {
	totals := map[string]int{}
	var weeks []string
	for _, d := range days {
		w := previousSunday(d.Date)
		if _, ok := totals[w]; !ok {
			weeks = append(weeks, w)
		}
		totals[w] += d.Count
	}
	byWeek := make([]Day, len(weeks))
	for i, w := range weeks {
		byWeek[i] = Day{w, totals[w]}
	}
	s, err := streaks(byWeek, func(string) bool { return false })
	s.Weekly = true
	// The first contribution is a day, not a week.
	s.FirstContribution = ""
	for _, d := range days {
		if d.Count > 0 {
			s.FirstContribution = d.Date
			break
		}
	}
	return s, err
}

// --------------------------------------------------------------- render

// streak themes: background, border, stroke, ring, fire, currStreakNum,
// sideNums, currStreakLabel, sideLabels, dates, excludeDaysLabel.
var streakThemes = loadThemes[map[string]string]("streak.json")

var cssName = regexp.MustCompile(`^[a-z]{3,20}$`)

type streakTheme struct {
	c        map[string]string
	gradient string
}

func streakThemeFrom(p Params) streakTheme {
	name := strings.ReplaceAll(strings.ToLower(p.Str("theme")), "_", "-")
	base, ok := streakThemes[name]
	if !ok {
		base = streakThemes["default"]
	}
	t := streakTheme{c: map[string]string{}}
	for k, v := range base {
		t.c[k] = v
		pv := strings.ToLower(p.Str(k))
		switch {
		case IsHex(pv):
			t.c[k] = "#" + pv
		case cssName.MatchString(pv):
			t.c[k] = pv
		case k == "background":
			if _, ok := ParseGradient(pv); ok {
				t.c[k] = pv
			}
		}
	}
	if p.Bool("hide_border", false) {
		t.c["border"] = "#0000"
	}
	if g, ok := ParseGradient(t.c["background"]); ok {
		t.c["background"] = "url(#gradient)"
		t.gradient = g.Def("gradient")
	}
	return t
}

var months = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

var bracketed = regexp.MustCompile(`\[.*?\]`)

// FormatDate shows "Oct 31" in the current year and "Oct 31, 2024" otherwise.
// A format uses PHP date() letters (d j D l m n M F y Y); text in [brackets]
// is only shown for other years, e.g. "M j[, Y]".
func FormatDate(date, format string, currentYear int) string {
	d := parseDate(date)
	sameYear := d.Year() == currentYear
	month := months[d.Month()-1]
	if format == "" {
		if sameYear {
			return fmt.Sprintf("%s %d", month[:3], d.Day())
		}
		return fmt.Sprintf("%s %d, %d", month[:3], d.Day(), d.Year())
	}
	pattern := strings.NewReplacer("[", "", "]", "").Replace(format)
	if sameYear {
		pattern = bracketed.ReplaceAllString(format, "")
	}
	var b strings.Builder
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch c {
		case '\\':
			if i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
			}
		case 'd':
			fmt.Fprintf(&b, "%02d", d.Day())
		case 'j':
			b.WriteString(strconv.Itoa(d.Day()))
		case 'D':
			b.WriteString(dayNames[d.Weekday()])
		case 'l':
			b.WriteString(d.Weekday().String())
		case 'm':
			fmt.Fprintf(&b, "%02d", int(d.Month()))
		case 'n':
			b.WriteString(strconv.Itoa(int(d.Month())))
		case 'M':
			b.WriteString(month[:3])
		case 'F':
			b.WriteString(month)
		case 'y':
			fmt.Fprintf(&b, "%02d", d.Year()%100)
		case 'Y':
			b.WriteString(strconv.Itoa(d.Year()))
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// splitLines puts text over two <tspan>s when longer than maxChars, preferring to split at " - ".
func splitLines(text string, maxChars, line1Offset int) string {
	lines := []string{text}
	if maxChars > 0 && len([]rune(text)) > maxChars {
		if strings.Contains(text, " - ") {
			lines = strings.Split(text, " - ")
			for i := 1; i < len(lines); i++ {
				lines[i] = "- " + lines[i]
			}
		} else {
			lines = []string{""}
			for _, w := range strings.Split(text, " ") {
				cur := lines[len(lines)-1]
				switch {
				case cur != "" && len(cur)+1+len(w) > maxChars:
					lines = append(lines, w)
				case cur != "":
					lines[len(lines)-1] = cur + " " + w
				default:
					lines[len(lines)-1] = w
				}
			}
		}
	}
	if len(lines) < 2 {
		return Esc(text)
	}
	return fmt.Sprintf(`<tspan x="0" dy="%d">%s</tspan><tspan x="0" dy="16">%s</tspan>`, line1Offset, Esc(lines[0]), Esc(strings.Join(lines[1:], " ")))
}

const streakFont = `font-family="'Segoe UI', Ubuntu, sans-serif"`

const firePath = "M 1.5 0.67 C 1.5 0.67 2.24 3.32 2.24 5.47 C 2.24 7.53 0.89 9.2 -1.17 9.2 C -3.23 9.2 -4.79 7.53 -4.79 5.47 L -4.76 5.11 C -6.78 7.51 -8 10.62 -8 13.99 C -8 18.41 -4.42 22 0 22 C 4.42 22 8 18.41 8 13.99 C 8 8.6 5.41 3.79 1.5 0.67 Z M -0.29 19 C -2.07 19 -3.51 17.6 -3.51 15.86 C -3.51 14.24 -2.46 13.1 -0.7 12.74 C 1.07 12.38 2.9 11.53 3.92 10.16 C 4.31 11.45 4.51 12.81 4.51 14.2 C 4.51 16.85 2.36 19 -0.29 19 Z"

func formatStreakNumber(n int, short bool) string {
	if !short {
		return Thousands(n)
	}
	units := []string{"", "K", "M", "B", "T"}
	v, i := float64(n), 0
	for ; v >= 1000 && i < len(units)-1; i++ {
		v /= 1000
	}
	return F(math.Round(v*10)/10) + units[i]
}

func dateRange(start, end string) string {
	if start == end {
		return start
	}
	return start + " - " + end
}

// RenderStreak draws the streak card. currentYear decides when dates show a year.
func RenderStreak(s StreakStats, p Params, currentYear int) string {
	t := streakThemeFrom(p)
	radius := F(p.NumOr("border_radius", 4.5))
	showTotal := !p.Bool("hide_total_contributions", false)
	showCurrent := !p.Bool("hide_current_streak", false)
	showLongest := !p.Bool("hide_longest_streak", false)
	columns := 0
	for _, b := range []bool{showTotal, showCurrent, showLongest} {
		if b {
			columns++
		}
	}
	width := float64(max(100*columns, p.IntOr("card_width", 495)))
	height := float64(max(170, p.IntOr("card_height", 195)))
	colWidth := 0.0
	if columns > 0 {
		colWidth = width / float64(columns)
	}
	dy := (height - 195) / 2
	short := p.Bool("short_numbers", false)
	dateFormat := p.Str("date_format")
	fmtDate := func(d string) string { return FormatDate(d, dateFormat, currentYear) }
	animations := !p.Bool("disable_animations", false)
	anim := func(delay float64) string {
		if !animations {
			return ""
		}
		return "opacity: 0; animation: fadein 0.5s linear forwards " + F(delay) + "s"
	}

	next := 0
	x := func(show bool) float64 {
		if !show {
			return -999
		}
		v := colWidth/2 + colWidth*float64(next)
		next++
		return v
	}
	xTotal, xCurrent, xLongest := x(showTotal), x(showCurrent), x(showLongest)

	labelChars, dateChars := 0, 0
	if columns > 0 {
		labelChars = int(width / float64(columns) / 7.5)
		dateChars = int(width / float64(columns) / 6)
	}
	first := ""
	if s.FirstContribution != "" {
		first = fmtDate(s.FirstContribution)
	}
	text := func(tx, ty float64, fill string, weight, size int, style, content string, y int) string {
		return fmt.Sprintf(`<g transform="translate(%s, %s)"><text x="0" y="%d" stroke-width="0" text-anchor="middle" fill="%s" stroke="none" %s font-weight="%d" font-size="%dpx" font-style="normal" style="%s">%s</text></g>`,
			F(tx), F(ty), y, fill, streakFont, weight, size, style, content)
	}
	c := t.c
	var body strings.Builder
	for i := range columns - 1 {
		bx := F(colWidth * float64(i+1))
		fmt.Fprintf(&body, `<line x1="%s" y1="%s" x2="%[1]s" y2="%[3]s" vector-effect="non-scaling-stroke" stroke-width="1" stroke="%[4]s" stroke-linejoin="miter" stroke-linecap="square" stroke-miterlimit="3"/>`,
			bx, F(28+dy/2), F(170+dy), c["stroke"])
	}
	currentLabel, longestLabel := "Current Streak", "Longest Streak"
	if s.Weekly {
		currentLabel, longestLabel = "Week Streak", "Longest Week Streak"
	}
	if showTotal {
		body.WriteString(text(xTotal, 48+dy, c["sideNums"], 700, 28, anim(0.6), formatStreakNumber(s.TotalContributions, short), 32))
		body.WriteString(text(xTotal, 84+dy, c["sideLabels"], 400, 14, anim(0.7), splitLines("Total Contributions", labelChars, -9), 32))
		body.WriteString(text(xTotal, 114+dy, c["dates"], 400, 12, anim(0.8), splitLines(first+" - Present", dateChars, 0), 32))
	}
	if showCurrent {
		numStyle := ""
		if animations {
			numStyle = "animation: currstreak 0.6s linear forwards"
		}
		body.WriteString(text(xCurrent, 108+dy, c["currStreakLabel"], 700, 14, anim(0.9), splitLines(currentLabel, labelChars, -9), 32))
		body.WriteString(text(xCurrent, 145+dy, c["dates"], 400, 12, anim(0.9), splitLines(dateRange(fmtDate(s.Current.Start), fmtDate(s.Current.End)), dateChars, 0), 21))
		fmt.Fprintf(&body, `<g mask="url(#mask_out_ring_behind_fire)"><circle cx="%s" cy="%s" r="40" fill="none" stroke="%s" stroke-width="5" style="%s"/></g>`,
			F(xCurrent), F(71+dy), c["ring"], anim(0.4))
		fmt.Fprintf(&body, `<g transform="translate(%s, %s)" stroke-opacity="0" style="%s"><path d="M -12 -0.5 L 15 -0.5 L 15 23.5 L -12 23.5 L -12 -0.5 Z" fill="none"/><path d="%s" fill="%s" stroke-opacity="0"/></g>`,
			F(xCurrent), F(19.5+dy), anim(0.6), firePath, c["fire"])
		body.WriteString(text(xCurrent, 48+dy, c["currStreakNum"], 700, 28, numStyle, formatStreakNumber(s.Current.Length, short), 32))
	}
	if showLongest {
		body.WriteString(text(xLongest, 48+dy, c["sideNums"], 700, 28, anim(1.2), formatStreakNumber(s.Longest.Length, short), 32))
		body.WriteString(text(xLongest, 84+dy, c["sideLabels"], 400, 14, anim(1.3), splitLines(longestLabel, labelChars, -9), 32))
		body.WriteString(text(xLongest, 114+dy, c["dates"], 400, 12, anim(1.4), splitLines(dateRange(fmtDate(s.Longest.Start), fmtDate(s.Longest.End)), dateChars, 0), 32))
	}
	if len(s.ExcludedDays) > 0 {
		fmt.Fprintf(&body, `<g transform="translate(5,187)"><text stroke-width="0" fill="%s" stroke="none" %s font-weight="400" font-size="10px" font-style="normal" style="%s">* Excluding %s</text></g>`,
			c["excludeDaysLabel"], streakFont, anim(0.9), Esc(strings.Join(s.ExcludedDays, ", ")))
	}

	title := fmt.Sprintf("%s contributions, current streak %d, longest streak %d", formatStreakNumber(s.TotalContributions, short), s.Current.Length, s.Longest.Length)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" style="isolation: isolate" viewBox="0 0 %[1]s %[2]s" width="%[1]spx" height="%[2]spx">
<title>%[3]s</title>
<style>
  @keyframes currstreak { 0%% { font-size: 3px; opacity: 0.2; } 80%% { font-size: 34px; opacity: 1; } 100%% { font-size: 28px; opacity: 1; } }
  @keyframes fadein { 0%% { opacity: 0; } 100%% { opacity: 1; } }
</style>
<defs>
  <clipPath id="outer_rectangle"><rect width="%[1]s" height="%[2]s" rx="%[4]s"/></clipPath>
  <mask id="mask_out_ring_behind_fire"><rect width="%[1]s" height="%[2]s" fill="white"/><ellipse cx="%[5]s" cy="32" rx="13" ry="18" fill="black"/></mask>
  %[6]s
</defs>
<g clip-path="url(#outer_rectangle)">
  <rect stroke="%[7]s" fill="%[8]s" rx="%[4]s" x="0.5" y="0.5" width="%[9]s" height="%[10]s"/>
  %[11]s
</g>
</svg>`, F(width), F(height), Esc(title), radius, F(xCurrent), t.gradient, c["border"], c["background"], F(width-1), F(height-1), body.String())
}

// StreakError is the streak app's error card: a sad face and the message, in the requested theme.
func StreakError(message string, p Params) string {
	t := streakThemeFrom(p)
	width := float64(max(300, p.IntOr("card_width", 495)))
	height := float64(max(170, p.IntOr("card_height", 195)))
	c := t.c
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" style="isolation: isolate" viewBox="0 0 %[1]s %[2]s" width="%[1]spx" height="%[2]spx">
<defs><clipPath id="outer_rectangle"><rect width="%[1]s" height="%[2]s" rx="%[3]s"/></clipPath>%[4]s</defs>
<g clip-path="url(#outer_rectangle)">
  <rect stroke="%[5]s" fill="%[6]s" rx="%[3]s" x="0.5" y="0.5" width="%[7]s" height="%[8]s"/>
  <g transform="translate(%[9]s, %[10]s)"><text x="0" y="50" dy="0.25em" stroke-width="0" text-anchor="middle" fill="%[11]s" stroke="none" %[12]s font-weight="400" font-size="14px">%[13]s</text></g>
  <g transform="translate(%[9]s, %[14]s)">
    <path fill="%[15]s" d="M0,35.8c-25.2,0-45.7,20.5-45.7,45.7s20.5,45.8,45.7,45.8s45.7-20.5,45.7-45.7S25.2,35.8,0,35.8z M0,122.3c-11.2,0-21.4-4.5-28.8-11.9c-2.9-2.9-5.4-6.3-7.4-10c-3-5.7-4.6-12.1-4.6-18.9c0-22.5,18.3-40.8,40.8-40.8 c10.7,0,20.4,4.1,27.7,10.9c3.8,3.5,6.9,7.7,9.1,12.4c2.6,5.3,4,11.3,4,17.6C40.8,104.1,22.5,122.3,0,122.3z"/>
    <path fill="%[15]s" d="M4.8,93.8c5.4,1.1,10.3,4.2,13.7,8.6l3.9-3c-4.1-5.3-10-9-16.6-10.4c-10.6-2.2-21.7,1.9-28.3,10.4l3.9,3 C-13.1,95.3-3.9,91.9,4.8,93.8z"/>
    <circle fill="%[15]s" cx="-15" cy="71" r="4.9"/><circle fill="%[15]s" cx="15" cy="71" r="4.9"/>
  </g>
</g>
</svg>`, F(width), F(height), F(p.NumOr("border_radius", 4.5)), t.gradient, c["border"], c["background"], F(width-1), F(height-1),
		F(width/2), F(height/2+10.5), c["sideLabels"], streakFont, Esc(message), F((height-195)/2), c["fire"])
}
