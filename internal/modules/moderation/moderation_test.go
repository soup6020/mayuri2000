package moderation

import (
	"testing"
)

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
