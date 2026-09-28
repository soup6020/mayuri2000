package when

import (
	"testing"
	"time"
)

// Wednesday 2026-09-30 14:00:00 in New York.
var (
	ny, _ = time.LoadLocation("America/New_York")
	now   = time.Date(2026, 9, 30, 14, 0, 0, 0, ny)
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"10":                    10 * time.Minute,
		"3h12m":                 3*time.Hour + 12*time.Minute,
		"1h30m":                 90 * time.Minute,
		"2d":                    48 * time.Hour,
		"1w":                    7 * 24 * time.Hour,
		"1 hour":                time.Hour,
		"2 hours and 5 minutes": 2*time.Hour + 5*time.Minute,
		"3days12h":              84 * time.Hour,
		"45s":                   45 * time.Second,
	} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "10x", "m10", "99999999999999d"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) should fail", bad)
		}
	}
}

func TestParse(t *testing.T) {
	at := func(day, h, m int) time.Time { return time.Date(2026, 9, day, h, m, 0, 0, ny) }
	for in, want := range map[string]time.Time{
		"3h12m":              now.Add(3*time.Hour + 12*time.Minute),
		"in 90 minutes":      now.Add(90 * time.Minute),
		"20:15":              at(30, 20, 15),
		"8:15pm":             at(30, 20, 15),
		"8 PM":               at(30, 20, 0),
		"9am":                time.Date(2026, 10, 1, 9, 0, 0, 0, ny), // already past today
		"13:59":              time.Date(2026, 10, 1, 13, 59, 0, 0, ny),
		"noon":               time.Date(2026, 10, 1, 12, 0, 0, 0, ny),
		"midnight":           time.Date(2026, 10, 1, 0, 0, 0, 0, ny),
		"12am":               time.Date(2026, 10, 1, 0, 0, 0, 0, ny),
		"12:30pm":            time.Date(2026, 10, 1, 12, 30, 0, 0, ny),
		"tomorrow":           time.Date(2026, 10, 1, 14, 0, 0, 0, ny),
		"tomorrow 9am":       time.Date(2026, 10, 1, 9, 0, 0, 0, ny),
		"tomorrow at 21:30":  time.Date(2026, 10, 1, 21, 30, 0, 0, ny),
		"today 18:00":        at(30, 18, 0),
		"friday":             time.Date(2026, 10, 2, 14, 0, 0, 0, ny),
		"fri 6pm":            time.Date(2026, 10, 2, 18, 0, 0, 0, ny),
		"next friday at 6pm": time.Date(2026, 10, 2, 18, 0, 0, 0, ny),
		"wednesday 9am":      time.Date(2026, 10, 7, 9, 0, 0, 0, ny), // today's slot passed
		"wednesday 4pm":      at(30, 16, 0),
		"2026-10-05 20:15":   time.Date(2026, 10, 5, 20, 15, 0, 0, ny),
		"2026-10-05T07:00":   time.Date(2026, 10, 5, 7, 0, 0, 0, ny),
	} {
		got, err := Parse(in, now, ny)
		if err != nil || !got.Equal(want) {
			t.Errorf("Parse(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "soon", "25:00", "13pm", "8:5", "2026-09-01 10:00", "2026-10-05", "today 9am", "blursday"} {
		if got, err := Parse(bad, now, ny); err == nil {
			t.Errorf("Parse(%q) should fail, got %v", bad, got)
		}
	}
}

func TestParseTimeZones(t *testing.T) {
	// 20:15 is interpreted in the given location, not the server's.
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	got, err := Parse("20:15", now, tokyo)
	if err != nil {
		t.Fatal(err)
	}
	if l := got.In(tokyo); l.Hour() != 20 || l.Minute() != 15 || !got.After(now) {
		t.Fatalf("got %v", got.In(tokyo))
	}
}

func TestSplit(t *testing.T) {
	for in, want := range map[string]struct {
		at   time.Time
		rest string
	}{
		"3h12m take out the laundry": {now.Add(3*time.Hour + 12*time.Minute), "take out the laundry"},
		"tomorrow 9am call mom":      {time.Date(2026, 10, 1, 9, 0, 0, 0, ny), "call mom"},
		"in 2 hours check the oven":  {now.Add(2 * time.Hour), "check the oven"},
		"20:15 dinner":               {time.Date(2026, 9, 30, 20, 15, 0, 0, ny), "dinner"},
		"friday at 6pm pub":          {time.Date(2026, 10, 2, 18, 0, 0, 0, ny), "pub"},
	} {
		at, rest, err := Split(in, now, ny)
		if err != nil || !at.Equal(want.at) || rest != want.rest {
			t.Errorf("Split(%q) = %v, %q, %v; want %v, %q", in, at, rest, err, want.at, want.rest)
		}
	}
	if _, _, err := Split("take out the laundry", now, ny); err == nil {
		t.Error("Split without a time should fail")
	}
}
