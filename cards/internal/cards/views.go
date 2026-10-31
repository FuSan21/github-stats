package cards

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	. "github.com/FuSan21/github-stats/cards/internal/render"
)

// Views is the profile view counter (replaces komarev.com/ghpvc). The count
// is kept in memory and flushed to a JSON file by the caller.
type Views struct {
	mu    sync.Mutex
	file  string
	count int
	dirty bool
}

// LoadViews reads the count from file, or starts at initial if there is none yet
// (on first start, carrying the count over from komarev).
func LoadViews(file string, initial int) *Views {
	v := &Views{file: file, count: initial, dirty: true}
	if raw, err := os.ReadFile(file); err == nil {
		var saved struct {
			Views int `json:"views"`
		}
		if json.Unmarshal(raw, &saved) == nil {
			v.count, v.dirty = saved.Views, false
		}
	}
	return v
}

// Count records a view and returns the new total.
func (v *Views) Count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.count++
	v.dirty = true
	return v.count
}

// Flush writes the count if it changed, atomically via a temporary file.
func (v *Views) Flush() error {
	v.mu.Lock()
	if !v.dirty {
		v.mu.Unlock()
		return nil
	}
	count := v.count
	v.dirty = false
	v.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(v.file), 0o755); err != nil {
		return err
	}
	tmp := v.file + ".tmp"
	if err := os.WriteFile(tmp, fmt.Appendf(nil, `{"views":%d}`, count), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, v.file)
}

var badgeColors = map[string]string{
	"brightgreen": "#4c1", "green": "#97ca00", "yellow": "#dfb317", "yellowgreen": "#a4a61d",
	"orange": "#fe7d37", "red": "#e05d44", "blue": "#007ec6", "grey": "#555", "lightgrey": "#9f9f9f",
}

var (
	narrowRe = regexp.MustCompile(`[il.,:;'|!]`)
	wideRe   = regexp.MustCompile(`[mwMW]`)
	upperRe  = regexp.MustCompile(`[A-Z0-9]`)
)

// badgeWidth roughly measures Verdana 11px, good enough to size a badge.
func badgeWidth(s string) float64 {
	w := 0.0
	for _, c := range s {
		switch ch := string(c); {
		case narrowRe.MatchString(ch):
			w += 3.2
		case wideRe.MatchString(ch):
			w += 9.5
		case upperRe.MatchString(ch):
			w += 7.5
		default:
			w += 6.4
		}
	}
	return w
}

var badgeHex = regexp.MustCompile(`^(?i)[0-9a-f]{3,8}$`)

// ViewsBadge draws the shields-style badge.
func ViewsBadge(count int, p Params) string {
	label := p.Str("label")
	if label == "" {
		label = "Profile views"
	}
	raw := p.Str("color")
	if raw == "" {
		raw = "blue"
	}
	color, ok := badgeColors[raw]
	if !ok {
		color = badgeColors["blue"]
		if badgeHex.MatchString(raw) {
			color = "#" + raw
		}
	}
	value := Thousands(count)
	lw := int(badgeWidth(label) + 12 + 0.5)
	vw := int(badgeWidth(value) + 12 + 0.5)
	w := lw + vw
	l := Esc(label)
	lx, vx := F(float64(lw)/2), F(float64(lw)+float64(vw)/2)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%[1]d" height="20" role="img" aria-label="%[2]s: %[3]s">
<title>%[2]s: %[3]s</title>
<linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
<clipPath id="r"><rect width="%[1]d" height="20" rx="3" fill="#fff"/></clipPath>
<g clip-path="url(#r)"><rect width="%[4]d" height="20" fill="#555"/><rect x="%[4]d" width="%[5]d" height="20" fill="%[6]s"/><rect width="%[1]d" height="20" fill="url(#s)"/></g>
<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
<text x="%[7]s" y="15" fill="#010101" fill-opacity=".3">%[2]s</text><text x="%[7]s" y="14">%[2]s</text>
<text x="%[8]s" y="15" fill="#010101" fill-opacity=".3">%[3]s</text><text x="%[8]s" y="14">%[3]s</text>
</g></svg>`, w, l, value, lw, vw, color, lx, vx)
}
