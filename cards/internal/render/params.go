package render

import (
	"math"
	"net/url"
	"strconv"
	"strings"
)

// Params gives typed access to query parameters, using the upstream apps' conventions.
type Params struct{ q url.Values }

func NewParams(q url.Values) Params { return Params{q} }

// Str is the parameter's value, or "" when missing or empty.
func (p Params) Str(name string) string { return p.q.Get(name) }

// Bool recognises only "true" and "false"; anything else is the fallback.
func (p Params) Bool(name string, fallback bool) bool {
	switch p.q.Get(name) {
	case "true":
		return true
	case "false":
		return false
	}
	return fallback
}

func (p Params) Num(name string) (float64, bool) {
	v := strings.TrimSpace(p.q.Get(name))
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, false
	}
	return n, true
}

func (p Params) NumOr(name string, fallback float64) float64 {
	if n, ok := p.Num(name); ok {
		return n
	}
	return fallback
}

func (p Params) Int(name string) (int, bool) {
	n, ok := p.Num(name)
	if !ok || math.Abs(n) > 1e9 {
		return 0, false
	}
	return int(n), true
}

func (p Params) IntOr(name string, fallback int) int {
	if n, ok := p.Int(name); ok {
		return n
	}
	return fallback
}

// List splits a comma-separated parameter, trimmed, empties dropped.
func (p Params) List(name string) []string {
	var out []string
	for _, s := range strings.Split(p.q.Get(name), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
