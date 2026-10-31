// Package render holds the SVG helpers, query parameters and card frame
// shared by every card.
package render

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

// Esc escapes text for SVG content and attribute values.
func Esc(s string) string { return escaper.Replace(s) }

// F formats a number the way JavaScript's String(n) does: shortest form, no trailing zeros.
func F(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Round2 rounds to two decimals, to keep coordinates short.
func Round2(v float64) float64 { return math.Round(v*100) / 100 }

func Clamp[T int | float64](v, lo, hi T) T { return min(max(v, lo), hi) }

// Truncate shortens s to max runes, ending with an ellipsis.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimRight(string(r[:max-1]), " \t\n") + "…"
}

// Thousands formats n with comma separators, like toLocaleString("en-US").
func Thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// ToFixed formats like JavaScript's toFixed.
func ToFixed(v float64, digits int) string { return strconv.FormatFloat(v, 'f', digits, 64) }

// KFormat turns 1234 into "1.2k", like readme-stats. A precision >= 0 forces that many decimals.
func KFormat(n int, precision int) string {
	if precision >= 0 {
		return ToFixed(float64(n)/1000, precision) + "k"
	}
	if n < 1000 && n > -1000 {
		return strconv.Itoa(n)
	}
	v, _ := strconv.ParseFloat(ToFixed(float64(n)/1000, 1), 64)
	return F(v) + "k"
}

func FormatBytes(bytes float64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := min(int(math.Floor(math.Log(bytes)/math.Log(1024))), len(units)-1)
	return fmt.Sprintf("%s %s", ToFixed(bytes/math.Pow(1024, float64(i)), 1), units[i])
}

// Per-character advance widths (in em) of the readme-stats font stack, from space to "~".
var widths = [...]float64{
	0.2796875, 0.2765625, 0.3546875, 0.5546875, 0.5546875, 0.8890625, 0.665625, 0.190625,
	0.3328125, 0.3328125, 0.3890625, 0.5828125, 0.2765625, 0.3328125, 0.2765625, 0.3015625,
	0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875, 0.5546875,
	0.5546875, 0.5546875, 0.2765625, 0.2765625, 0.584375, 0.5828125, 0.584375, 0.5546875,
	1.0140625, 0.665625, 0.665625, 0.721875, 0.721875, 0.665625, 0.609375, 0.7765625,
	0.721875, 0.2765625, 0.5, 0.665625, 0.5546875, 0.8328125, 0.721875, 0.7765625,
	0.665625, 0.7765625, 0.721875, 0.665625, 0.609375, 0.721875, 0.665625, 0.94375,
	0.665625, 0.665625, 0.609375, 0.2765625, 0.3546875, 0.2765625, 0.4765625, 0.5546875,
	0.3328125, 0.5546875, 0.5546875, 0.5, 0.5546875, 0.5546875, 0.2765625, 0.5546875,
	0.5546875, 0.221875, 0.240625, 0.5, 0.221875, 0.8328125, 0.5546875, 0.5546875,
	0.5546875, 0.5546875, 0.3328125, 0.5, 0.2765625, 0.5546875, 0.5, 0.721875,
	0.5, 0.5, 0.5, 0.3546875, 0.259375, 0.353125, 0.5890625,
}

// MeasureText estimates the rendered width of s at fontSize px.
func MeasureText(s string, fontSize float64) float64 {
	w := 0.0
	for _, c := range s {
		if i := int(c) - 32; i >= 0 && i < len(widths) {
			w += widths[i]
		} else {
			w += 0.5279276315789471
		}
	}
	return w * fontSize
}

// Flex wraps each non-empty item in a <g> offset by gap along a row or column.
func Flex(items []string, gap float64, column bool) string {
	var b strings.Builder
	i := 0
	for _, item := range items {
		if item == "" {
			continue
		}
		x, y := float64(i)*gap, 0.0
		if column {
			x, y = 0, float64(i)*gap
		}
		fmt.Fprintf(&b, `<g transform="translate(%s, %s)">%s</g>`, F(x), F(y), item)
		i++
	}
	return b.String()
}

func Chunk[T any](s []T, size int) [][]T {
	var out [][]T
	for size > 0 && len(s) > 0 {
		n := min(size, len(s))
		out = append(out, s[:n])
		s = s[n:]
	}
	return out
}

var hexRe = regexp.MustCompile(`^(?i)([0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$`)

func IsHex(s string) bool { return hexRe.MatchString(s) }

// HexColor is "#" + s for a bare hex value, else "".
func HexColor(s string) string {
	if IsHex(s) {
		return "#" + s
	}
	return ""
}

// Gradient is a `angle,hex,hex[,hex...]` background, as readme-stats and streak accept.
type Gradient struct {
	Angle int
	Stops []string
}

var angleRe = regexp.MustCompile(`^-?\d+$`)

func ParseGradient(s string) (Gradient, bool) {
	parts := strings.Split(s, ",")
	if len(parts) < 3 || !angleRe.MatchString(parts[0]) {
		return Gradient{}, false
	}
	g := Gradient{}
	g.Angle, _ = strconv.Atoi(parts[0])
	for _, c := range parts[1:] {
		if !IsHex(c) {
			return Gradient{}, false
		}
		g.Stops = append(g.Stops, "#"+c)
	}
	return g, true
}

func (g Gradient) Def(id string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<linearGradient id="%s" gradientTransform="rotate(%d)" gradientUnits="userSpaceOnUse">`, id, g.Angle)
	for i, c := range g.Stops {
		fmt.Fprintf(&b, `<stop offset="%s%%" stop-color="%s"/>`, F(float64(i)*100/float64(len(g.Stops)-1)), c)
	}
	b.WriteString("</linearGradient>")
	return b.String()
}
