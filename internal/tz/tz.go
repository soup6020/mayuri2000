// Package tz resolves and searches IANA time zone names.
package tz

//go:generate go run gen.go

import (
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // embed the tz database so lookups work without system zoneinfo
)

var byLower = func() map[string]string {
	m := make(map[string]string, len(Names))
	for _, n := range Names {
		m[strings.ToLower(n)] = n
	}
	return m
}()

// normalize lowercases input and treats spaces as underscores, so
// "america/new york" matches "America/New_York".
func normalize(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "_"))
}

// Lookup resolves a zone name case-insensitively.
func Lookup(name string) (string, *time.Location, bool) {
	canonical, ok := byLower[normalize(name)]
	if !ok {
		return "", nil, false
	}
	loc, err := time.LoadLocation(canonical)
	if err != nil {
		return "", nil, false
	}
	return canonical, loc, true
}

// Search returns up to limit zone names matching query, best matches first:
// exact match, full-name prefix ("america/new"), city prefix ("new_y"), then
// substring ("york").
func Search(query string, limit int) []string {
	q := normalize(query)
	type match struct {
		name  string
		score int
	}
	var matches []match
	for _, n := range Names {
		l := strings.ToLower(n)
		city := l[strings.LastIndex(l, "/")+1:]
		score := -1
		switch {
		case q == "":
			score = 3
		case l == q || city == q:
			score = 0
		case strings.HasPrefix(l, q):
			score = 1
		case strings.HasPrefix(city, q):
			score = 2
		case strings.Contains(l, q):
			score = 3
		}
		if score < 0 {
			continue
		}
		// Prefer Area/Location names over legacy aliases like "EST5EDT".
		if !strings.Contains(n, "/") {
			score += 4
		}
		matches = append(matches, match{n, score})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score < matches[j].score })
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.name
	}
	return out
}
