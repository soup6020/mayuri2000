package core

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestKeyPermissions(t *testing.T) {
	if got := KeyPermissions(discord.PermissionAdministrator | discord.PermissionBanMembers); got != "Administrator (all permissions)" {
		t.Errorf("admin: %q", got)
	}
	// Order must be stable regardless of map iteration.
	p := discord.PermissionKickMembers | discord.PermissionManageGuild | discord.PermissionSendMessages
	for i := 0; i < 20; i++ {
		if got := KeyPermissions(p); got != "Manage Server, Kick Members" {
			t.Fatalf("got %q", got)
		}
	}
	if got := KeyPermissions(discord.PermissionSendMessages); got != "None" {
		t.Errorf("no key perms: %q", got)
	}
}

func TestJoinLimited(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = "<@&123456789012345678>"
	}
	out := joinLimited(items, " ", 1024)
	if len(out) > 1024 || !strings.HasSuffix(out, "more") {
		t.Fatalf("len %d: %q", len(out), out[len(out)-30:])
	}
	if got := joinLimited([]string{"a", "b"}, " ", 1024); got != "a b" {
		t.Fatalf("got %q", got)
	}
}

func TestManagedBy(t *testing.T) {
	bot := snowflake.ID(5)
	for want, r := range map[string]discord.Role{
		"":                {},
		"Bot <@5>":        {Managed: true, Tags: &discord.RoleTag{BotID: &bot}},
		"Server Boosting": {Managed: true, Tags: &discord.RoleTag{PremiumSubscriber: true}},
		"Integration":     {Managed: true},
	} {
		if got := managedBy(r); got != want {
			t.Errorf("managedBy(%+v) = %q, want %q", r, got, want)
		}
	}
}
