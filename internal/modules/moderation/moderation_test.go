package moderation

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"10":       10 * time.Minute,
		"10m":      10 * time.Minute,
		"1h30m":    90 * time.Minute,
		"2d":       48 * time.Hour,
		"1w":       7 * 24 * time.Hour,
		"1 hour":   time.Hour,
		"3days12h": 84 * time.Hour,
		"45s":      45 * time.Second,
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

func TestParseIDs(t *testing.T) {
	ids, invalid := ParseIDs("123456789012345678, <@234567890123456789> 123456789012345678 nope")
	if len(ids) != 2 || len(invalid) != 1 || invalid[0] != "nope" {
		t.Fatalf("ids=%v invalid=%v", ids, invalid)
	}
}

func TestSplitHackbanArgs(t *testing.T) {
	ids, days, reason := splitHackbanArgs("123456789012345678 234567890123456789, 3 raiding the server")
	if ids != "123456789012345678 234567890123456789," || days != 3 || reason != "raiding the server" {
		t.Fatalf("got %q %d %q", ids, days, reason)
	}
	ids, days, reason = splitHackbanArgs("123456789012345678")
	if ids != "123456789012345678" || days != 0 || reason != "No reason given" {
		t.Fatalf("got %q %d %q", ids, days, reason)
	}
	_, days, reason = splitHackbanArgs("123456789012345678 spam bot")
	if days != 0 || reason != "spam bot" {
		t.Fatalf("got %d %q", days, reason)
	}
}
