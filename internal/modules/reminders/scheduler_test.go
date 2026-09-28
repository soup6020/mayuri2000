package reminders

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/store"
)

type sent struct {
	channel snowflake.ID
	msg     discord.MessageCreate
}

type fakeRest struct {
	mu       sync.Mutex
	sent     []sent
	dmErr    error
	postErr  map[snowflake.ID]error
	dmChanID snowflake.ID
}

func (f *fakeRest) CreateMessage(ch snowflake.ID, msg discord.MessageCreate, _ ...rest.RequestOpt) (*discord.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ch == f.dmChanID && f.dmErr != nil {
		return nil, f.dmErr
	}
	if err := f.postErr[ch]; err != nil {
		return nil, err
	}
	f.sent = append(f.sent, sent{ch, msg})
	return &discord.Message{}, nil
}

func (f *fakeRest) CreateDMChannel(snowflake.ID, ...rest.RequestOpt) (*discord.DMChannel, error) {
	var dm discord.DMChannel
	_ = dm.UnmarshalJSON([]byte(`{"id":"` + f.dmChanID.String() + `","type":1}`))
	return &dm, nil
}

func setup(t *testing.T) (*Scheduler, *fakeRest, *store.Store, *time.Time) {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fr := &fakeRest{postErr: map[snowflake.ID]error{}, dmChanID: 999}
	s := NewScheduler(st, fr, slog.New(slog.DiscardHandler))
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, fr, st, &now
}

func add(t *testing.T, st *store.Store, r store.Reminder) store.Reminder {
	t.Helper()
	r, err := st.AddReminder(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func pending(t *testing.T, st *store.Store, user snowflake.ID) []store.Reminder {
	t.Helper()
	rs, err := st.UserReminders(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestDeliverDMAndChannel(t *testing.T) {
	s, fr, st, now := setup(t)
	created := now.Add(-time.Hour)
	add(t, st, store.Reminder{UserID: 1, GuildID: 5, SourceChannelID: 50, Message: "stretch @everyone", DueAt: *now, CreatedAt: created})
	add(t, st, store.Reminder{UserID: 2, GuildID: 5, ChannelID: 60, SourceChannelID: 50, Message: "standup", DueAt: now.Add(-time.Second), CreatedAt: created})
	add(t, st, store.Reminder{UserID: 3, Message: "later", DueAt: now.Add(time.Hour), CreatedAt: created})

	s.DeliverDue(context.Background())
	if len(fr.sent) != 2 {
		t.Fatalf("sent %d messages, want 2: %+v", len(fr.sent), fr.sent)
	}
	channelMsg, dm := fr.sent[0], fr.sent[1] // oldest due first
	if channelMsg.channel != 60 || !strings.HasPrefix(channelMsg.msg.Content, "<@2> ⏰ **Reminder:** standup") {
		t.Fatalf("channel reminder: %+v", channelMsg)
	}
	if am := channelMsg.msg.AllowedMentions; am == nil || len(am.Users) != 1 || am.Users[0] != 2 || len(am.Parse) != 0 {
		t.Fatalf("channel reminder may only ping its owner: %+v", am)
	}
	if dm.channel != 999 || !strings.Contains(dm.msg.Content, "stretch @everyone") || !strings.Contains(dm.msg.Content, "in <#50>") {
		t.Fatalf("dm reminder: %+v", dm)
	}
	if am := dm.msg.AllowedMentions; am == nil || len(am.Users)+len(am.Parse)+len(am.Roles) != 0 {
		t.Fatalf("DM must not ping anyone: %+v", am)
	}
	if len(pending(t, st, 1))+len(pending(t, st, 2)) != 0 || len(pending(t, st, 3)) != 1 {
		t.Fatal("delivered reminders should be deleted and future ones kept")
	}
}

func TestDMClosedFallsBackToChannel(t *testing.T) {
	s, fr, st, now := setup(t)
	fr.dmErr = &rest.Error{Code: rest.JSONErrorCodeCannotSendMessagesToThisUser}
	add(t, st, store.Reminder{UserID: 1, GuildID: 5, SourceChannelID: 50, Message: "hi", DueAt: *now, CreatedAt: *now})
	s.DeliverDue(context.Background())
	if len(fr.sent) != 1 || fr.sent[0].channel != 50 || !strings.Contains(fr.sent[0].msg.Content, "couldn't DM you") {
		t.Fatalf("fallback: %+v", fr.sent)
	}
	if len(pending(t, st, 1)) != 0 {
		t.Fatal("reminder not deleted after fallback delivery")
	}
}

func TestRetryAndGiveUp(t *testing.T) {
	s, fr, st, now := setup(t)
	fr.postErr[60] = errors.New("discord 500")
	r := add(t, st, store.Reminder{UserID: 1, GuildID: 5, ChannelID: 60, Message: "x", DueAt: *now, CreatedAt: *now})

	s.DeliverDue(context.Background())
	rs := pending(t, st, 1)
	if len(rs) != 1 || rs[0].Attempts != 1 || !rs[0].DueAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("after first failure: %+v", rs)
	}
	for i := 0; i < maxAttempts; i++ {
		*now = now.Add(time.Hour)
		s.DeliverDue(context.Background())
	}
	if len(pending(t, st, 1)) != 0 {
		t.Fatalf("reminder %d should be dropped after %d attempts", r.ID, maxAttempts)
	}

	// Permanent errors drop the reminder immediately.
	fr.postErr[61] = &rest.Error{Code: rest.JSONErrorCodeUnknownChannel}
	add(t, st, store.Reminder{UserID: 2, GuildID: 5, ChannelID: 61, Message: "y", DueAt: *now, CreatedAt: *now})
	s.DeliverDue(context.Background())
	if len(pending(t, st, 2)) != 0 {
		t.Fatal("reminder for a deleted channel should be dropped")
	}
}

func TestBodyLateNotice(t *testing.T) {
	due := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	r := store.Reminder{Message: "m", DueAt: due, CreatedAt: due.Add(-time.Hour)}
	if strings.Contains(Body(r, due.Add(time.Minute)), "late") {
		t.Error("a minute late shouldn't be flagged")
	}
	if !strings.Contains(Body(r, due.Add(time.Hour)), "sorry, this is late") {
		t.Error("an hour late should be flagged")
	}
}

func TestRunWakesForNewReminders(t *testing.T) {
	s, fr, st, _ := setup(t)
	s.now = time.Now
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	add(t, st, store.Reminder{UserID: 1, GuildID: 5, ChannelID: 60, Message: "soon", DueAt: time.Now().Add(200 * time.Millisecond), CreatedAt: time.Now()})
	s.Wake()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		fr.mu.Lock()
		n := len(fr.sent)
		fr.mu.Unlock()
		if n == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scheduler did not deliver a reminder added while running")
}
