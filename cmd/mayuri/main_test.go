package main

import (
	"log/slog"
	"path/filepath"
	"testing"

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

func TestCommandsValid(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := cmd.NewRegistry(slog.Default(), st, "%")
	r.Add(core.Commands()...)
	r.AddMenus(core.UserInfoMenu())
	nsb := notsobot.New(imagefind.New(""), &imaging.Magick{}, &imaging.FFmpeg{})
	cmds := nsb.Commands()
	r.Add(cmds...)
	r.AddMenus(nsb.Menus(cmds)...)
	r.Add(moderation.Commands()...)
	r.Add(search.New("").Commands()...)
	r.Add(feeds.New(rss.NewFetcher()).Commands()...)
	r.Add(reminders.New(reminders.NewScheduler(nil, nil, slog.Default())).Commands()...)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d commands", len(r.Commands()))

	if err := st.SetPrefix(t.Context(), 1, "!"); err != nil {
		t.Fatal(err)
	}
	gid := cmdID(1)
	if p := r.Prefix(&gid); p != "!" {
		t.Fatalf("prefix override = %q", p)
	}
	_ = st.SetPrefix(t.Context(), 1, "")
	if p := r.Prefix(&gid); p != "%" {
		t.Fatalf("prefix reset = %q", p)
	}
}

func cmdID(n uint64) snowflake.ID { return snowflake.ID(n) }
