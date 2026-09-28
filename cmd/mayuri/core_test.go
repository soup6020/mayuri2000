package main

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/imagefind"
	"mayuri2000/internal/imaging"
	"mayuri2000/internal/modules/core"
	"mayuri2000/internal/modules/moderation"
	"mayuri2000/internal/modules/notsobot"
	"mayuri2000/internal/modules/search"
	"mayuri2000/internal/store"
)

func testRegistry(t *testing.T) *cmd.Registry {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := cmd.NewRegistry(slog.Default(), st, "%")
	r.Add(core.Commands()...)
	nsb := notsobot.New(imagefind.New(""), &imaging.Magick{})
	cmds := nsb.Commands()
	r.Add(cmds...)
	r.AddMenus(nsb.Menus(cmds)...)
	r.Add(moderation.Commands()...)
	r.Add(search.New("").Commands()...)
	return r
}

func runCmd(t *testing.T, r *cmd.Registry, name string, guild *snowflake.ID, perms discord.Permissions, args map[string]any) (*cmd.Recorder, error) {
	t.Helper()
	command, ok := r.Lookup(name)
	if !ok {
		t.Fatalf("no command %s", name)
	}
	c, rec := cmd.NewTestContext(r, args)
	c.GuildID = guild
	c.Permissions = perms
	c.Author = discord.User{ID: 42, Username: "tester"}
	return rec, command.Run(c)
}

func TestHelpPages(t *testing.T) {
	r := testRegistry(t)
	rec, err := runCmd(t, r, "help", nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := rec.Messages[0]
	if len(m.Embeds) != 1 || len(m.Components) != 1 {
		t.Fatalf("unexpected help message: %+v", m)
	}
	seen := map[string]bool{}
	for _, c := range m.Components[0].(discord.ActionRowComponent).Components {
		id := c.(discord.ButtonComponent).CustomID
		if seen[id] {
			t.Fatalf("duplicate custom ID %s", id)
		}
		seen[id] = true
	}
	if d := m.Embeds[0].Description; len(d) == 0 || len([]rune(d)) > 4096 {
		t.Fatalf("bad description length %d", len(d))
	}
}

func TestTwoPagePaginatorUniqueIDs(t *testing.T) {
	r := testRegistry(t)
	c, rec := cmd.NewTestContext(r, nil)
	if err := c.Paginate([]cmd.Page{{Content: "a"}, {Content: "b"}}); err != nil {
		t.Fatal(err)
	}
	row := rec.Messages[0].Components[0].(discord.ActionRowComponent).Components
	if row[0].(discord.ButtonComponent).CustomID == row[1].(discord.ButtonComponent).CustomID {
		t.Fatal("prev and next buttons share a custom ID")
	}
}

func TestAliases(t *testing.T) {
	r := testRegistry(t)
	guild := snowflake.ID(1)
	manage := discord.PermissionManageGuild

	if _, err := runCmd(t, r, "alias", &guild, 0, map[string]any{"action": "add", "name": "mg", "command": "magik 5"}); err == nil {
		t.Fatal("adding an alias without Manage Server should fail")
	}
	if _, err := runCmd(t, r, "alias", &guild, manage, map[string]any{"action": "add", "name": "mg", "command": "%cas 5"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]any{
		{"action": "add", "name": "ban", "command": "magik"}, // shadows a command
		{"action": "add", "name": "x", "command": "nothing"}, // unknown target
		{"action": "add", "name": "y", "command": "mg"},      // alias to alias
		{"action": "add", "name": "two words", "command": "ping"},
	} {
		if _, err := runCmd(t, r, "alias", &guild, manage, bad); err == nil {
			t.Fatalf("expected error for %v", bad)
		}
	}

	command, args, ok := r.Resolve(&guild, "MG https://x/y.png")
	if !ok || command.Name != "magik" || args != "5 https://x/y.png" {
		t.Fatalf("alias resolved to %v %q %v", command, args, ok)
	}
	if _, _, ok := r.Resolve(nil, "mg"); ok {
		t.Fatal("aliases must not resolve in DMs")
	}
	other := snowflake.ID(2)
	if _, _, ok := r.Resolve(&other, "mg"); ok {
		t.Fatal("aliases must be per-guild")
	}

	rec, err := runCmd(t, r, "alias", &guild, 0, map[string]any{"action": "list"})
	if err != nil || !strings.Contains(rec.Messages[0].Embeds[0].Description, "`%mg` → `%magik 5`") {
		t.Fatalf("list: %v %+v", err, rec.Messages)
	}
	rec, err = runCmd(t, r, "help", &guild, 0, map[string]any{"command": "mg"})
	if err != nil || !strings.Contains(rec.Messages[0].Content, "alias for `%magik 5`") {
		t.Fatalf("help for alias: %v %+v", err, rec.Messages)
	}
	if _, err := runCmd(t, r, "alias", &guild, manage, map[string]any{"action": "remove", "name": "mg"}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := r.Resolve(&guild, "mg"); ok {
		t.Fatal("alias still resolves after removal")
	}
}

func TestAliasesPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.db")
	st, _ := store.Open(path)
	if err := st.SetAlias(5, store.Alias{Name: "a", Command: "ping", CreatorID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.IncrementCommand("ping"); err != nil {
		t.Fatal(err)
	}
	_ = st.IncrementCommand("ping")
	st.Close()

	st, _ = store.Open(path)
	defer st.Close()
	if a, ok := st.Alias(5, "a"); !ok || a.Command != "ping" {
		t.Fatalf("alias not persisted: %+v", a)
	}
	counts, _ := st.CommandCounts()
	if len(counts) != 1 || counts[0].Count != 2 {
		t.Fatalf("counts not persisted: %+v", counts)
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[string]string{"45s": "45s", "1h2m3s": "1h 2m 3s", "50h": "2d 2h 0m 0s"} {
		dur, _ := time.ParseDuration(d)
		if got := core.FormatDuration(dur); got != want {
			t.Errorf("FormatDuration(%s) = %q, want %q", d, got, want)
		}
	}
}
