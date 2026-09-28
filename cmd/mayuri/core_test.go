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
	"mayuri2000/internal/modules/feeds"
	"mayuri2000/internal/modules/moderation"
	"mayuri2000/internal/modules/notsobot"
	"mayuri2000/internal/modules/reminders"
	"mayuri2000/internal/modules/search"
	"mayuri2000/internal/rss"
	"mayuri2000/internal/store"
)

func testRegistry(t *testing.T) *cmd.Registry {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := cmd.NewRegistry(slog.Default(), st, "%")
	r.Add(core.Commands()...)
	r.AddMenus(core.UserInfoMenu())
	nsb := notsobot.New(imagefind.New(""), &imaging.Magick{})
	cmds := nsb.Commands()
	r.Add(cmds...)
	r.AddMenus(nsb.Menus(cmds)...)
	r.Add(moderation.Commands()...)
	r.Add(search.New("").Commands()...)
	r.Add(feeds.New(rss.NewFetcher()).Commands()...)
	r.Add(reminders.New(reminders.NewScheduler(nil, nil, slog.Default())).Commands()...)
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
	ctx := t.Context()
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAlias(ctx, 5, store.Alias{Name: "a", Command: "ping", CreatorID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.IncrementCommand(ctx, "ping"); err != nil {
		t.Fatal(err)
	}
	_ = st.IncrementCommand(ctx, "ping")
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if a, ok := st.Alias(5, "a"); !ok || a.Command != "ping" {
		t.Fatalf("alias not persisted: %+v", a)
	}
	counts, _ := st.CommandCounts(ctx)
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

func TestTimeCommand(t *testing.T) {
	r := testRegistry(t)
	rec, err := runCmd(t, r, "time", nil, 0, map[string]any{"zone": "asia/kolkata"})
	if err != nil {
		t.Fatal(err)
	}
	out := rec.Messages[0].Content
	if !strings.Contains(out, "`Asia/Kolkata`") || !strings.Contains(out, "UTC+05:30") || !strings.Contains(out, "<t:") {
		t.Fatalf("unexpected output: %s", out)
	}
	t.Log(out)

	rec, err = runCmd(t, r, "time", nil, 0, nil)
	if err != nil || !strings.Contains(rec.Messages[0].Content, "`UTC`") {
		t.Fatalf("default zone: %v %+v", err, rec.Messages)
	}

	_, err = runCmd(t, r, "time", nil, 0, map[string]any{"zone": "london"})
	if err == nil || !strings.Contains(err.Error(), "Europe/London") {
		t.Fatalf("expected a suggestion for london, got %v", err)
	}
}

func TestFormatTime(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	got := core.FormatTime(time.Date(2026, 7, 4, 14, 5, 9, 0, ny))
	want := "**14:05:09** (2:05 PM) · Saturday, 4 July 2026 · EDT (UTC-04:00)"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	utc := core.FormatTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !strings.HasSuffix(utc, "· UTC+00:00") {
		t.Fatalf("utc: %q", utc)
	}
}

func TestHelpForGroups(t *testing.T) {
	r := testRegistry(t)
	rec, err := runCmd(t, r, "help", nil, 0, map[string]any{"command": "rss"})
	if err != nil || !strings.Contains(rec.Messages[0].Embeds[0].Description, "`%rss add <name> <url> [channel]`") {
		t.Fatalf("help rss: %v %+v", err, rec.Messages)
	}
	rec, err = runCmd(t, r, "help", nil, 0, map[string]any{"command": "rss template"})
	if err != nil || rec.Messages[0].Embeds[0].Title != "/rss template" {
		t.Fatalf("help rss template: %v %+v", err, rec.Messages)
	}
	if _, err := runCmd(t, r, "help", nil, 0, map[string]any{"command": "rss nope"}); err == nil {
		t.Fatal("unknown subcommand should error")
	}
}

func TestBase64Commands(t *testing.T) {
	r := testRegistry(t)
	group, _ := r.Lookup("b64")
	enc, _ := group.Sub("encode")
	dec, _ := group.Sub("d")

	c, rec := cmd.NewTestContext(r, map[string]any{"text": "hello world"})
	if err := enc.Run(c); err != nil || rec.Messages[0].Content != "```\naGVsbG8gd29ybGQ=\n```" {
		t.Fatalf("encode: %v %+v", err, rec.Messages)
	}
	c, rec = cmd.NewTestContext(r, map[string]any{"text": "a+b?", "urlsafe": true})
	if err := enc.Run(c); err != nil || rec.Messages[0].Content != "```\nYStiPw\n```" {
		t.Fatalf("urlsafe encode: %v %+v", err, rec.Messages)
	}
	c, rec = cmd.NewTestContext(r, map[string]any{"text": "aGVsbG8gd29ybGQ"})
	if err := dec.Run(c); err != nil || rec.Messages[0].Content != "```\nhello world\n```" {
		t.Fatalf("decode: %v %+v", err, rec.Messages)
	}
	// Binary output (here a PNG signature) is attached with a sensible name.
	c, rec = cmd.NewTestContext(r, map[string]any{"text": "iVBORw0KGgoAAAAA"})
	if err := dec.Run(c); err != nil || len(rec.Messages[0].Files) != 1 || rec.Messages[0].Files[0].Name != "decoded.png" {
		t.Fatalf("binary decode: %v %+v", err, rec.Messages)
	}
	// Output that would break out of the code block is attached instead.
	c, rec = cmd.NewTestContext(r, map[string]any{"text": "YGBgIHBpbmcgQGV2ZXJ5b25l"}) // "``` ping @everyone"
	if err := dec.Run(c); err != nil || len(rec.Messages[0].Files) != 1 {
		t.Fatalf("code fence escape: %v %+v", err, rec.Messages)
	}
	c, _ = cmd.NewTestContext(r, nil)
	if err := enc.Run(c); err == nil {
		t.Fatal("encode with no input should error")
	}
}

func TestRemindCommands(t *testing.T) {
	r := testRegistry(t)
	remind, _ := r.Lookup("remind")
	group, _ := r.Lookup("reminders")
	list, _ := group.Sub("list")
	cancel, _ := group.Sub("cancel")
	tzCmd, _ := r.Lookup("timezone")

	run := func(command *cmd.Command, user snowflake.ID, args map[string]any, prefix bool) (*cmd.Recorder, error) {
		c, rec := cmd.NewTestContext(r, args)
		c.Author = discord.User{ID: user}
		c.ChannelID = 77
		if prefix {
			c.SourceMessage = &discord.Message{}
		}
		return rec, command.Run(c)
	}

	rec, err := run(remind, 1, map[string]any{"when": "3h12m", "message": "laundry"}, false)
	if err != nil || !strings.Contains(rec.Messages[0].Content, "by DM <t:") || strings.Contains(rec.Messages[0].Content, "UTC") {
		t.Fatalf("delay reminder: %v %+v", err, rec.Messages)
	}
	// Clock times without a saved zone get the UTC hint.
	rec, err = run(remind, 1, map[string]any{"when": "20:15", "message": "dinner"}, false)
	if err != nil || !strings.Contains(rec.Messages[0].Content, "read as UTC") {
		t.Fatalf("clock reminder: %v %+v", err, rec.Messages)
	}
	// Prefix form splits the time off the front of the message.
	rec, err = run(remind, 1, map[string]any{"when": "tomorrow", "message": "9am call mom"}, true)
	if err != nil || !strings.Contains(rec.Messages[0].Content, "remind you by DM") {
		t.Fatalf("prefix reminder: %v %+v", err, rec.Messages)
	}
	for _, bad := range []map[string]any{
		{"when": "22d", "message": "too far"},
		{"when": "5s", "message": "too soon"},
		{"when": "someday", "message": "vague"},
	} {
		if _, err := run(remind, 1, bad, false); err == nil {
			t.Errorf("expected error for %v", bad)
		}
	}
	if _, err := run(remind, 1, map[string]any{"when": "1h", "message": "x", "channel": snowflake.ID(5)}, false); err == nil {
		t.Error("channel reminders in DMs should fail")
	}

	rec, err = run(list, 1, nil, false)
	if err != nil || !strings.Contains(rec.Messages[0].Embeds[0].Description, "laundry") || !strings.Contains(rec.Messages[0].Embeds[0].Description, "call mom") {
		t.Fatalf("list: %v %+v", err, rec.Messages)
	}
	if _, err := run(cancel, 2, map[string]any{"id": 1}, false); err == nil {
		t.Fatal("users must not cancel other people's reminders")
	}
	if _, err := run(cancel, 1, map[string]any{"id": 1}, false); err != nil {
		t.Fatalf("cancel own reminder: %v", err)
	}
	rec, _ = run(list, 1, nil, false)
	if strings.Contains(rec.Messages[0].Embeds[0].Description, "laundry") {
		t.Fatal("cancelled reminder still listed")
	}

	// Saving a zone removes the UTC hint and is used by /time.
	if _, err := run(tzCmd, 1, map[string]any{"zone": "asia/tokyo"}, false); err != nil {
		t.Fatal(err)
	}
	rec, _ = run(remind, 1, map[string]any{"when": "20:15", "message": "dinner"}, false)
	if strings.Contains(rec.Messages[0].Content, "read as UTC") {
		t.Fatal("UTC hint shown despite saved zone")
	}
	timeCmd, _ := r.Lookup("time")
	rec, _ = run(timeCmd, 1, nil, false)
	if !strings.Contains(rec.Messages[0].Content, "`Asia/Tokyo`") {
		t.Fatalf("/time should default to the saved zone: %+v", rec.Messages)
	}
	if _, err := run(tzCmd, 1, map[string]any{"zone": "mars"}, false); err == nil {
		t.Fatal("unknown zone should fail")
	}
}
