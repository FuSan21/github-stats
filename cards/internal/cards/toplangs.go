package cards

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/FuSan21/github-stats/cards/internal/gh"
	. "github.com/FuSan21/github-stats/cards/internal/render"
)

type RepoLang struct {
	Name  string
	Color string // "" when GitHub has none
	Size  float64
}

type RepoLanguages struct {
	Repo  string
	Langs []RepoLang
}

type Lang struct {
	Name  string
	Color string
	Size  float64
}

const langsQuery = `
query ($login: String!, $after: String) {
  user(login: $login) {
    repositories(ownerAffiliations: OWNER, isFork: false, first: 100, after: $after) {
      nodes {
        name
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
          edges { size node { color name } }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// FetchRepoLanguages loads the languages of every owned, non-fork repository (all pages).
func FetchRepoLanguages(ctx context.Context, c *gh.Client, cache *gh.Cache, login string) ([]RepoLanguages, error) {
	return gh.Cached(cache, "langs:"+login, gh.DataTTL, func() ([]RepoLanguages, error) {
		var repos []RepoLanguages
		var after *string
		for {
			var data struct {
				User struct {
					Repositories struct {
						Nodes []struct {
							Name      string `json:"name"`
							Languages struct {
								Edges []struct {
									Size float64 `json:"size"`
									Node struct {
										Name  string  `json:"name"`
										Color *string `json:"color"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"languages"`
						} `json:"nodes"`
						PageInfo pageInfo `json:"pageInfo"`
					} `json:"repositories"`
				} `json:"user"`
			}
			if err := c.GraphQL(ctx, langsQuery, map[string]any{"login": login, "after": after}, &data); err != nil {
				return nil, err
			}
			for _, r := range data.User.Repositories.Nodes {
				rl := RepoLanguages{Repo: r.Name}
				for _, e := range r.Languages.Edges {
					color := ""
					if e.Node.Color != nil {
						color = *e.Node.Color
					}
					rl.Langs = append(rl.Langs, RepoLang{e.Node.Name, color, e.Size})
				}
				repos = append(repos, rl)
			}
			pi := data.User.Repositories.PageInfo
			if !pi.HasNextPage {
				return repos, nil
			}
			after = pi.EndCursor
		}
	})
}

// TopLanguages ranks languages by bytes^sizeWeight * repos^countWeight, like
// readme-stats. The defaults (1, 0) rank purely by bytes.
func TopLanguages(repos []RepoLanguages, excludeRepos []string, sizeWeight, countWeight float64) []Lang {
	type total struct {
		color string
		size  float64
		count int
	}
	totals := map[string]*total{}
	var order []string
	for _, r := range repos {
		if slices.Contains(excludeRepos, r.Repo) {
			continue
		}
		for _, l := range r.Langs {
			t := totals[l.Name]
			if t == nil {
				t = &total{color: l.Color}
				totals[l.Name] = t
				order = append(order, l.Name)
			}
			t.size += l.Size
			t.count++
		}
	}
	langs := make([]Lang, 0, len(order))
	for _, name := range order {
		t := totals[name]
		color := t.color
		if color == "" {
			color = "#858585"
		}
		langs = append(langs, Lang{name, color, math.Pow(t.size, sizeWeight) * math.Pow(float64(t.count), countWeight)})
	}
	slices.SortStableFunc(langs, func(a, b Lang) int { return cmp.Compare(b.Size, a.Size) })
	return langs
}

// --------------------------------------------------------------- render

const (
	langsDefaultWidth = 300
	langsMinWidth     = 280
	langsPadding      = 25
	compactBaseHeight = 90
)

var layouts = []string{"normal", "compact", "donut", "donut-vertical", "pie"}

func langValue(size, total float64, format string) string {
	if format == "bytes" {
		return FormatBytes(size)
	}
	return ToFixed(size/total*100, 2) + "%"
}

func polar(cx, cy, r, deg float64) (float64, float64) {
	rad := deg * math.Pi / 180
	return cx + r*math.Cos(rad), cy + r*math.Sin(rad)
}

func compactLangNode(l Lang, total float64, i int, hideProgress bool, format string) string {
	value := ""
	if !hideProgress {
		value = langValue(l.Size, total, format)
	}
	return fmt.Sprintf(`<g class="stagger" style="animation-delay: %dms">
  <circle cx="5" cy="6" r="5" fill="%s"/>
  <text x="15" y="10" class="lang-name">%s %s</text>
</g>`, (i+3)*150, l.Color, Esc(l.Name), value)
}

// langColumns lays out two columns of dot + name + value.
func langColumns(langs []Lang, total float64, hideProgress bool, format string) string {
	var columns []string
	for _, col := range Chunk(langs, (len(langs)+1)/2) {
		nodes := make([]string, len(col))
		for i, l := range col {
			nodes[i] = compactLangNode(l, total, i, hideProgress, format)
		}
		columns = append(columns, Flex(nodes, 25, true))
	}
	longest := langs[0]
	for _, l := range langs {
		if len(l.Name) > len(longest.Name) {
			longest = l
		}
	}
	gap := 20 + MeasureText(longest.Name+" "+ToFixed(longest.Size/total*100, 2)+"%", 11)
	return Flex(columns, max(gap, 150), false)
}

func normalLayout(langs []Lang, width, total float64, format string) string {
	barWidth := width - 95
	nodes := make([]string, len(langs))
	for i, l := range langs {
		delay := (i + 3) * 150
		nodes[i] = fmt.Sprintf(`<g class="stagger" style="animation-delay: %dms">
  <text x="2" y="15" class="lang-name">%s</text>
  <text x="%s" y="34" class="lang-name">%s</text>
  <svg width="%s" x="0" y="25">
    <rect rx="5" ry="5" x="0" y="0" width="%[5]s" height="8" fill="#ddd"/>
    <svg width="%s%%"><rect height="8" fill="%s" rx="5" ry="5" x="0" y="0" class="lang-progress" style="animation-delay: %dms;"/></svg>
  </svg>
</g>`, delay, Esc(l.Name), F(barWidth+10), langValue(l.Size, total, format), F(barWidth),
			F(Clamp(l.Size/total*100, 2, 100)), l.Color, delay+300)
	}
	return Flex(nodes, 40, true)
}

func compactLayout(langs []Lang, width, total float64, hideProgress bool, format string) string {
	columns := langColumns(langs, total, hideProgress, format)
	if hideProgress {
		return `<g transform="translate(0, 0)">` + columns + `</g>`
	}
	barWidth := width - 50
	offset := 0.0
	var bar strings.Builder
	for _, l := range langs {
		w, _ := strconv.ParseFloat(ToFixed(l.Size/total*barWidth, 2), 64)
		shown := w
		if w < 10 {
			shown = w + 10
		}
		fmt.Fprintf(&bar, `<rect mask="url(#rect-mask)" x="%s" y="0" width="%s" height="8" fill="%s"/>`, F(offset), F(shown), l.Color)
		offset += w
	}
	return fmt.Sprintf(`<mask id="rect-mask"><rect x="0" y="0" width="%s" height="8" fill="white" rx="5"/></mask>%s
<g transform="translate(0, 25)">%s</g>`, F(barWidth), bar.String(), columns)
}

func donutLayout(langs []Lang, width, total float64, format string) string {
	cx := width / 3
	r := cx - 60
	var paths strings.Builder
	if len(langs) == 1 {
		fmt.Fprintf(&paths, `<circle cx="%s" cy="%[1]s" r="%s" stroke="%s" fill="none" stroke-width="12"/>`, F(cx), F(r), langs[0].Color)
	} else {
		pcts := make([]float64, len(langs))
		sum := 0.0
		for i, l := range langs {
			pcts[i], _ = strconv.ParseFloat(ToFixed(l.Size/total*100, 2), 64)
			sum += pcts[i]
		}
		start := 0.0
		for i, l := range langs {
			pct, _ := strconv.ParseFloat(ToFixed(pcts[i]/sum*100, 2), 64)
			end := start + 3.6*pct
			ax, ay := polar(cx, cx, r, end-90)
			bx, by := polar(cx, cx, r, start-90)
			large := 0
			if end-start > 180 {
				large = 1
			}
			start = end
			fmt.Fprintf(&paths, `<g class="stagger" style="animation-delay: %dms"><path d="M %s %s A %s %[4]s 0 %d 0 %s %s" stroke="%s" fill="none" stroke-width="12"/></g>`,
				(i+3)*100+300, F(ax), F(ay), F(r), large, F(bx), F(by), l.Color)
		}
	}
	nodes := make([]string, len(langs))
	for i, l := range langs {
		nodes[i] = compactLangNode(l, total, i, false, format)
	}
	return fmt.Sprintf(`<g>%s</g>
<g transform="translate(125, %d)"><svg width="%s" height="%[3]s">%s</svg></g>`,
		Flex(nodes, 32, true), -45+max(len(langs)-5, 0)*16, F(width), paths.String())
}

func donutVerticalLayout(langs []Lang, total float64, format string) string {
	const r = 80.0
	circumference := 2 * math.Pi * r
	indent := 0.0
	var circles strings.Builder
	for i, l := range langs {
		fmt.Fprintf(&circles, `<g class="stagger" style="animation-delay: %dms"><circle cx="150" cy="100" r="%s" fill="transparent" stroke="%s" stroke-width="25" stroke-dasharray="%s" stroke-dashoffset="%s"/></g>`,
			(i+1)*100, F(r), l.Color, F(circumference), F(indent))
		indent += circumference * (l.Size / total)
	}
	return fmt.Sprintf(`<svg><g><svg>%s</svg></g>
<g transform="translate(0, 220)"><svg x="%d">%s</svg></g></svg>`, circles.String(), langsPadding, langColumns(langs, total, false, format))
}

func pieLayout(langs []Lang, total float64, format string) string {
	const r, cx, cy = 90.0, 150.0, 100.0
	var paths strings.Builder
	if len(langs) == 1 {
		fmt.Fprintf(&paths, `<circle cx="%s" cy="%s" r="%s" stroke="none" fill="%s"/>`, F(cx), F(cy), F(r), langs[0].Color)
	} else {
		start := 0.0
		for i, l := range langs {
			angle := l.Size / total * 360
			ax, ay := polar(cx, cy, r, start)
			bx, by := polar(cx, cy, r, start+angle)
			start += angle
			large := 0
			if angle > 180 {
				large = 1
			}
			fmt.Fprintf(&paths, `<g class="stagger" style="animation-delay: %dms"><path d="M %s %s L %s %s A %s %[6]s 0 %d 1 %s %s Z" fill="%s"/></g>`,
				(i+1)*100, F(cx), F(cy), F(ax), F(ay), F(r), large, F(bx), F(by), l.Color)
		}
	}
	return fmt.Sprintf(`<svg><g><svg>%s</svg></g>
<g transform="translate(0, 220)"><svg x="%d">%s</svg></g></svg>`, paths.String(), langsPadding, langColumns(langs, total, false, format))
}

// RenderTopLangs draws the languages card in the requested layout.
func RenderTopLangs(all []Lang, p Params) string {
	colors := Colors(p)
	frame := FrameFrom(p)
	layout := p.Str("layout")
	if !slices.Contains(layouts, layout) {
		layout = "normal"
	}
	hideProgress := p.Bool("hide_progress", false)
	format := "percentages"
	if p.Str("stats_format") == "bytes" {
		format = "bytes"
	}

	defaultCount := 6
	if !(layout == "compact" || hideProgress) && (layout == "donut" || layout == "normal") {
		defaultCount = 5
	}
	hidden := p.List("hide")
	for i := range hidden {
		hidden[i] = strings.ToLower(hidden[i])
	}
	count := Clamp(p.IntOr("langs_count", defaultCount), 1, 20)
	var langs []Lang
	for _, l := range all {
		if !slices.Contains(hidden, strings.ToLower(l.Name)) && len(langs) < count {
			langs = append(langs, l)
		}
	}
	total := 0.0
	for _, l := range langs {
		total += l.Size
	}

	width := float64(max(p.IntOr("card_width", langsDefaultWidth), langsMinWidth))
	half := float64(int(math.Round(float64(len(langs)) / 2)))
	var height float64
	var body string
	switch {
	case len(langs) == 0:
		height = compactBaseHeight
		x := 0
		if layout == "pie" || layout == "donut-vertical" {
			x = langsPadding
		}
		body = fmt.Sprintf(`<text x="%d" y="11" class="stat bold" fill="%s">No languages data.</text>`, x, colors.Text)
	case layout == "pie":
		height = 300 + half*25
		body = pieLayout(langs, total, format)
	case layout == "donut-vertical":
		height = 300 + half*25
		body = donutVerticalLayout(langs, total, format)
	case layout == "compact" || hideProgress:
		height = compactBaseHeight + half*25
		if hideProgress {
			height -= 25
		}
		body = compactLayout(langs, width, total, hideProgress, format)
	case layout == "donut":
		height = 215 + float64(max(len(langs)-5, 0)*32)
		width += 50
		body = donutLayout(langs, width, total, format)
	default:
		height = 45 + float64(len(langs)+1)*40
		body = normalLayout(langs, width, total, format)
	}

	css := fmt.Sprintf(`
  @keyframes slideInAnimation { from { width: 0; } to { width: calc(100%%-100px); } }
  @keyframes growWidthAnimation { from { width: 0; } to { width: 100%%; } }
  .stat { font: 600 14px 'Segoe UI', Ubuntu, "Helvetica Neue", Sans-Serif; fill: %[1]s; }
  @supports(-moz-appearance: auto) { .stat { font-size: 12px; } }
  .bold { font-weight: 700 }
  .lang-name { font: 400 11px "Segoe UI", Ubuntu, Sans-Serif; fill: %[1]s; }
  .stagger { opacity: 0; animation: fadeInAnimation 0.3s ease-in-out forwards; }
  #rect-mask rect { animation: slideInAnimation 1s ease-in-out forwards; }
  .lang-progress { animation: growWidthAnimation 0.6s ease-in-out forwards; }`, colors.Text)

	if layout != "pie" && layout != "donut-vertical" {
		body = fmt.Sprintf(`<svg x="%d">%s</svg>`, langsPadding, body)
	}
	desc := make([]string, len(langs))
	for i, l := range langs {
		desc[i] = l.Name + ": " + langValue(l.Size, total, format)
	}
	return card{
		Frame: frame, Width: width, Height: height, Title: frame.Title("Most Used Languages"),
		Colors: colors, CSS: css, Desc: strings.Join(desc, ", "),
	}.render(body)
}
