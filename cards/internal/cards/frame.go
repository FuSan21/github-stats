// Package cards renders each card: its GitHub query, data shaping and SVG.
package cards

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/FuSan21/github-stats/cards/internal/render"
)

//go:embed themes/*.json
var themeFiles embed.FS

func loadThemes[T any](name string) map[string]T {
	raw, err := themeFiles.ReadFile("themes/" + name)
	if err != nil {
		panic(err)
	}
	var out map[string]T
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("themes/%s: %v", name, err))
	}
	return out
}

// readme-stats themes: title_color, icon_color, text_color, bg_color, border_color (bare hex).
var statsThemes = loadThemes[map[string]string]("readme-stats.json")

// CardColors are the resolved colours of a readme-stats style card.
type CardColors struct {
	Title, Text, Icon, Ring, Border string
	Bg                              string    // a colour, unless BgGradient is set
	BgGradient                      *Gradient // card background gradient
}

// Colors resolves the theme with per-colour query overrides, falling back to the default theme.
func Colors(p Params) CardColors {
	base := statsThemes["default"]
	theme, ok := statsThemes[p.Str("theme")]
	if !ok {
		theme = base
	}
	pick := func(key, fallback string) string {
		if c := HexColor(p.Str(key)); c != "" {
			return c
		}
		if c := HexColor(theme[key]); c != "" {
			return c
		}
		return fallback
	}
	title := pick("title_color", "#"+base["title_color"])
	border := base["border_color"]
	if b, ok := theme["border_color"]; ok {
		border = b
	}
	c := CardColors{
		Title:  title,
		Text:   pick("text_color", "#"+base["text_color"]),
		Icon:   pick("icon_color", "#"+base["icon_color"]),
		Ring:   pick("ring_color", title),
		Border: pick("border_color", "#"+border),
	}
	bg := p.Str("bg_color")
	if g, ok := ParseGradient(bg); ok {
		c.BgGradient = &g
	} else if h := HexColor(bg); h != "" {
		c.Bg = h
	} else if h := HexColor(theme["bg_color"]); h != "" {
		c.Bg = h
	} else {
		c.Bg = "#" + base["bg_color"]
	}
	return c
}

// Frame holds the options every readme-stats card reads from the query.
type Frame struct {
	CustomTitle  string
	HideTitle    bool
	HideBorder   bool
	BorderRadius float64
	Animations   bool
}

func FrameFrom(p Params) Frame {
	return Frame{
		CustomTitle:  p.Str("custom_title"),
		HideTitle:    p.Bool("hide_title", false),
		HideBorder:   p.Bool("hide_border", false),
		BorderRadius: p.NumOr("border_radius", 4.5),
		Animations:   !p.Bool("disable_animations", false),
	}
}

// Title is the custom title if given, else def.
func (f Frame) Title(def string) string {
	if f.CustomTitle != "" {
		return f.CustomTitle
	}
	return def
}

type card struct {
	Frame
	Width, Height   float64
	Title           string
	Colors          CardColors
	CSS             string
	A11yTitle, Desc string
}

const paddingX, paddingY = 25, 35

func (c card) render(body string) string {
	height := c.Height
	if c.HideTitle {
		height -= 30
	}
	fill, defs := c.Colors.Bg, ""
	if g := c.Colors.BgGradient; g != nil {
		fill, defs = "url(#gradient)", "<defs>"+g.Def("gradient")+"</defs>"
	}
	a11y := c.A11yTitle
	if a11y == "" {
		a11y = c.Title
	}
	noAnim := ""
	if !c.Animations {
		noAnim = "* { animation-duration: 0s !important; animation-delay: 0s !important; }"
	}
	title, bodyY := "", paddingX
	if !c.HideTitle {
		title = fmt.Sprintf(`<g transform="translate(%d, %d)">%s</g>`, paddingX, paddingY,
			Flex([]string{`<text x="0" y="0" class="header">` + Esc(c.Title) + `</text>`}, 25, false))
		bodyY = paddingY + 20
	}
	stroke := 1
	if c.HideBorder {
		stroke = 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg width="%s" height="%s" viewBox="0 0 %[1]s %[2]s" fill="none" xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="descId">
<title id="titleId">%s</title>
<desc id="descId">%s</desc>
<style>
  .header { font: 600 18px 'Segoe UI', Ubuntu, Sans-Serif; fill: %s; animation: fadeInAnimation 0.8s ease-in-out forwards; }
  @supports(-moz-appearance: auto) { .header { font-size: 15.5px; } }
  %s
  @keyframes scaleInAnimation { from { transform: translate(-5px, 5px) scale(0); } to { transform: translate(-5px, 5px) scale(1); } }
  @keyframes fadeInAnimation { from { opacity: 0; } to { opacity: 1; } }
  %s
</style>
%s
<rect x="0.5" y="0.5" rx="%s" height="99%%" width="%s" stroke="%s" fill="%s" stroke-opacity="%d"/>
%s
<g transform="translate(0, %d)">
%s
</g>
</svg>`, F(c.Width), F(height), Esc(a11y), Esc(c.Desc), c.Colors.Title, c.CSS, noAnim, defs,
		F(c.BorderRadius), F(c.Width-1), c.Colors.Border, fill, stroke, title, bodyY, body)
	return b.String()
}

// ErrorCard is a readme-stats style error card.
func ErrorCard(message string, c CardColors) string {
	fill := c.Bg
	if c.BgGradient != nil {
		fill = c.BgGradient.Stops[0]
	}
	return fmt.Sprintf(`<svg width="576.5" height="120" viewBox="0 0 576.5 120" fill="%s" xmlns="http://www.w3.org/2000/svg">
<style>
  .text { font: 600 16px 'Segoe UI', Ubuntu, Sans-Serif; fill: %s }
  .small { font: 600 12px 'Segoe UI', Ubuntu, Sans-Serif; fill: %s }
</style>
<rect x="0.5" y="0.5" width="575.5" height="99%%" rx="4.5" fill="%[1]s" stroke="%[4]s"/>
<text x="25" y="45" class="text">Something went wrong!</text>
<text x="25" y="70" class="small">%[5]s</text>
</svg>`, fill, c.Title, c.Text, c.Border, Esc(message))
}
