// Package when parses human-friendly durations and points in time, such as
// "3h12m", "20:15", "8:15pm", "tomorrow 9am" or "friday at noon".
package when

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses durations like "10m", "1h30m", "2d", "1w",
// "2 hours 5 minutes" or a bare number of minutes.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "and", "")
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if n, err := parseUint(s); err == nil {
		return time.Duration(n) * time.Minute, nil
	}
	units := map[string]time.Duration{
		"s": time.Second, "sec": time.Second, "secs": time.Second, "second": time.Second, "seconds": time.Second,
		"m": time.Minute, "min": time.Minute, "mins": time.Minute, "minute": time.Minute, "minutes": time.Minute,
		"h": time.Hour, "hr": time.Hour, "hrs": time.Hour, "hour": time.Hour, "hours": time.Hour,
		"d": 24 * time.Hour, "day": 24 * time.Hour, "days": 24 * time.Hour,
		"w": 7 * 24 * time.Hour, "week": 7 * 24 * time.Hour, "weeks": 7 * 24 * time.Hour,
	}
	var total time.Duration
	for s != "" {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		n, err := parseUint(s[:i])
		if err != nil {
			return 0, err
		}
		s = s[i:]
		j := 0
		for j < len(s) && (s[j] < '0' || s[j] > '9') {
			j++
		}
		unit, ok := units[s[:j]]
		if !ok {
			return 0, fmt.Errorf("unknown unit %q", s[:j])
		}
		s = s[j:]
		total += time.Duration(n) * unit
		if total > 365*24*time.Hour {
			return 0, errors.New("duration too long")
		}
	}
	return total, nil
}

func parseUint(s string) (int64, error) {
	var n int64
	if s == "" || len(s) > 9 {
		return 0, errors.New("bad number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("bad number")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

// parseClock parses "20:15", "20:15:30", "8:15pm", "8pm", "8 pm", "noon" and
// "midnight" into hours, minutes and seconds.
func parseClock(s string) (h, m, sec int, ok bool) {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	s = strings.ReplaceAll(s, ".", "")
	switch s {
	case "noon", "midday":
		return 12, 0, 0, true
	case "midnight":
		return 0, 0, 0, true
	}
	meridiem := ""
	for _, suffix := range []string{"am", "pm", "a", "p"} {
		if strings.HasSuffix(s, suffix) {
			meridiem = suffix[:1]
			s = strings.TrimSuffix(s, suffix)
			break
		}
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 || (len(parts) == 1 && meridiem == "") {
		return 0, 0, 0, false // a bare number is a duration, not a time
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" || len(p) > 2 || (i > 0 && len(p) != 2) {
			return 0, 0, 0, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	h, m, sec = nums[0], nums[1], nums[2]
	if m > 59 || sec > 59 {
		return 0, 0, 0, false
	}
	switch meridiem {
	case "":
		if h > 23 {
			return 0, 0, 0, false
		}
	default:
		if h < 1 || h > 12 {
			return 0, 0, 0, false
		}
		if meridiem == "a" && h == 12 {
			h = 0
		} else if meridiem == "p" && h != 12 {
			h += 12
		}
	}
	return h, m, sec, true
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday,
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "tues": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
}

// Parse resolves s to a point in time after now. Clock times and dates are
// interpreted in loc. Supported forms:
//
//	3h12m, 90m, 2 hours, in 45 minutes      (durations)
//	20:15, 8:15pm, 8pm, noon, midnight      (next occurrence)
//	tomorrow, tomorrow 9am, tomorrow at 9am
//	friday, friday 18:00, next friday at 6pm
//	2026-10-01 20:15, 2026-10-01T20:15
func Parse(s string, now time.Time, loc *time.Location) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "in ")
	s = strings.TrimPrefix(s, "at ")
	if s == "" {
		return time.Time{}, errors.New("no time given")
	}
	if d, err := ParseDuration(s); err == nil {
		if d <= 0 {
			return time.Time{}, errors.New("duration must be positive")
		}
		return now.Add(d), nil
	}

	local := now.In(loc)

	// ISO date with time: 2026-10-01 20:15 or 2026-10-01T20:15.
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		date, err := time.ParseInLocation("2006-01-02", s[:10], loc)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date %q", s[:10])
		}
		rest := strings.TrimLeft(s[10:], " t")
		rest = strings.TrimPrefix(rest, "at ")
		if rest == "" {
			return time.Time{}, errors.New("add a time to the date, e.g. 2026-10-01 20:15")
		}
		h, m, sec, ok := parseClock(rest)
		if !ok {
			return time.Time{}, fmt.Errorf("invalid time %q", rest)
		}
		t := time.Date(date.Year(), date.Month(), date.Day(), h, m, sec, 0, loc)
		if !t.After(now) {
			return time.Time{}, errors.New("that time is in the past")
		}
		return t, nil
	}

	// Optional day word followed by an optional clock time.
	day, clock := "", s
	if first, rest, found := strings.Cut(s, " "); found || weekdayOrRelative(first) {
		if weekdayOrRelative(first) {
			day, clock = first, strings.TrimSpace(rest)
		} else if first == "next" {
			if d, r, _ := strings.Cut(rest, " "); weekdayOrRelative(d) {
				day, clock = d, strings.TrimSpace(r)
			}
		}
	}
	clock = strings.TrimSpace(strings.TrimPrefix(clock, "at "))

	h, m, sec := local.Hour(), local.Minute(), local.Second()
	hasClock := clock != ""
	if hasClock {
		var ok bool
		if h, m, sec, ok = parseClock(clock); !ok {
			return time.Time{}, fmt.Errorf("can't understand %q", s)
		}
	} else if day == "" {
		return time.Time{}, fmt.Errorf("can't understand %q", s)
	}

	base := time.Date(local.Year(), local.Month(), local.Day(), h, m, sec, 0, loc)
	switch {
	case day == "today":
		// base as-is
	case day == "tomorrow":
		base = base.AddDate(0, 0, 1)
	case day != "":
		wd := weekdays[day]
		days := (int(wd) - int(local.Weekday()) + 7) % 7
		if days == 0 && !base.After(now) {
			days = 7
		}
		base = base.AddDate(0, 0, days)
	default:
		// Bare clock time: the next time it occurs.
		if !base.After(now) {
			base = base.AddDate(0, 0, 1)
		}
	}
	if !base.After(now) {
		return time.Time{}, errors.New("that time is in the past")
	}
	return base, nil
}

func weekdayOrRelative(s string) bool {
	_, ok := weekdays[s]
	return ok || s == "today" || s == "tomorrow"
}

// Split finds the longest leading run of words in text that Parse accepts
// and returns the time and the remaining text. It lets prefix commands
// accept "tomorrow 9am take out the trash" without quoting.
func Split(text string, now time.Time, loc *time.Location) (time.Time, string, error) {
	words := strings.Fields(text)
	var firstErr error
	for n := min(len(words), 5); n >= 1; n-- {
		t, err := Parse(strings.Join(words[:n], " "), now, loc)
		if err == nil {
			return t, strings.Join(words[n:], " "), nil
		}
		if n == 1 {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = errors.New("no time given")
	}
	return time.Time{}, text, firstErr
}
