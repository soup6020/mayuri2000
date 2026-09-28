package tz

import (
	"slices"
	"testing"
)

func TestLookup(t *testing.T) {
	for in, want := range map[string]string{
		"America/New_York": "America/New_York",
		"america/new_york": "America/New_York",
		"utc":              "UTC",
		"US/Eastern":       "US/Eastern",
		"asia/kolkata":     "Asia/Kolkata",
	} {
		name, loc, ok := Lookup(in)
		if !ok || name != want || loc == nil {
			t.Errorf("Lookup(%q) = %q, %v, %v; want %q", in, name, loc, ok, want)
		}
	}
	if name, _, ok := Lookup("  America/New York "); !ok || name != "America/New_York" {
		t.Errorf("surrounding spaces not trimmed: %q", name)
	}
	for _, bad := range []string{"", "Mars/Olympus_Mons", "../../etc/passwd", "local", "Local"} {
		if name, _, ok := Lookup(bad); ok {
			t.Errorf("Lookup(%q) unexpectedly resolved to %q", bad, name)
		}
	}
}

func TestSearch(t *testing.T) {
	if got := Search("new_y", 5); len(got) == 0 || got[0] != "America/New_York" {
		t.Errorf("city prefix: got %v", got)
	}
	if got := Search("europe/lon", 5); len(got) == 0 || got[0] != "Europe/London" {
		t.Errorf("full prefix: got %v", got)
	}
	if got := Search("tokyo", 5); !slices.Contains(got, "Asia/Tokyo") {
		t.Errorf("exact city: got %v", got)
	}
	if got := Search("", 25); len(got) != 25 {
		t.Errorf("empty query should fill the list, got %d", len(got))
	}
	if got := Search("zzzzzz", 25); len(got) != 0 {
		t.Errorf("nonsense query matched %v", got)
	}
	for _, n := range Search("", 1000) {
		if len(n) > 100 {
			t.Errorf("zone name too long for a Discord choice: %s", n)
		}
	}
}

func TestAllNamesLoad(t *testing.T) {
	for _, n := range Names {
		if _, _, ok := Lookup(n); !ok {
			t.Errorf("zone %q does not load", n)
		}
	}
}
