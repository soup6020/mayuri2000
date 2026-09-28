package reminders

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/store"
)

const maxAttempts = 5

// Messenger is the part of the REST client the scheduler needs.
type Messenger interface {
	CreateMessage(channelID snowflake.ID, msg discord.MessageCreate, opts ...rest.RequestOpt) (*discord.Message, error)
	CreateDMChannel(userID snowflake.ID, opts ...rest.RequestOpt) (*discord.DMChannel, error)
}

// Scheduler delivers reminders when they come due. Reminders live in the
// database, so ones that came due while the bot was offline are delivered
// (marked late) on startup.
type Scheduler struct {
	Store *store.Store
	Rest  Messenger
	Log   *slog.Logger

	wake chan struct{}
	now  func() time.Time
}

func NewScheduler(st *store.Store, r Messenger, log *slog.Logger) *Scheduler {
	return &Scheduler{Store: st, Rest: r, Log: log, wake: make(chan struct{}, 1), now: time.Now}
}

// Wake makes the scheduler re-check the next due time, e.g. after a new
// reminder is added.
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run delivers reminders until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		s.DeliverDue(ctx)
		wait := time.Minute
		if next, ok, err := s.Store.NextReminderDue(ctx); err == nil && ok {
			wait = min(wait, max(0, next.Sub(s.now())))
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// DeliverDue sends every reminder that is due now.
func (s *Scheduler) DeliverDue(ctx context.Context) {
	due, err := s.Store.DueReminders(ctx, s.now())
	if err != nil {
		s.Log.Error("reminders: loading due reminders", "err", err)
		return
	}
	for _, r := range due {
		err := s.deliver(r)
		switch {
		case err == nil:
			_ = s.Store.DeleteReminder(ctx, r.ID, 0)
		case permanent(err) || r.Attempts+1 >= maxAttempts:
			s.Log.Warn("reminders: giving up on reminder", "id", r.ID, "user", r.UserID, "err", err)
			_ = s.Store.DeleteReminder(ctx, r.ID, 0)
		default:
			s.Log.Warn("reminders: delivery failed, retrying", "id", r.ID, "err", err)
			_ = s.Store.RetryReminder(ctx, r.ID, s.now().Add(time.Minute<<r.Attempts))
		}
	}
}

func permanent(err error) bool {
	return rest.IsJSONErrorCode(err,
		rest.JSONErrorCodeUnknownChannel,
		rest.JSONErrorCodeUnknownUser,
		rest.JSONErrorCodeMissingAccess,
		rest.JSONErrorCodeLackPermissionsToPerformAction,
		rest.JSONErrorCodeCannotSendMessagesToThisUser,
	)
}

// Body renders the reminder text (without the leading mention).
func Body(r store.Reminder, now time.Time) string {
	msg := fmt.Sprintf("⏰ **Reminder:** %s\n-# Set <t:%d:R>", r.Message, r.CreatedAt.Unix())
	if r.ChannelID == 0 && r.SourceChannelID != 0 && r.GuildID != 0 {
		msg += fmt.Sprintf(" in <#%s>", r.SourceChannelID)
	}
	if late := now.Sub(r.DueAt); late > 2*time.Minute {
		msg += fmt.Sprintf(" · sorry, this is late (was due <t:%d:R>)", r.DueAt.Unix())
	}
	return msg
}

func (s *Scheduler) deliver(r store.Reminder) error {
	body := Body(r, s.now())
	// Only ever ping the reminder's owner, whatever the message contains.
	onlyOwner := &discord.AllowedMentions{Users: []snowflake.ID{r.UserID}}

	if r.ChannelID != 0 {
		_, err := s.Rest.CreateMessage(r.ChannelID, discord.MessageCreate{
			Content: fmt.Sprintf("<@%s> %s", r.UserID, body), AllowedMentions: onlyOwner,
		})
		return err
	}

	dm, err := s.Rest.CreateDMChannel(r.UserID)
	if err == nil {
		_, err = s.Rest.CreateMessage(dm.ID(), discord.MessageCreate{Content: body, AllowedMentions: &discord.AllowedMentions{}})
	}
	if err != nil && rest.IsJSONErrorCode(err, rest.JSONErrorCodeCannotSendMessagesToThisUser) && r.GuildID != 0 && r.SourceChannelID != 0 {
		// DMs are closed: fall back to the channel the reminder was set in.
		_, err = s.Rest.CreateMessage(r.SourceChannelID, discord.MessageCreate{
			Content:         fmt.Sprintf("<@%s> I couldn't DM you, so here's your reminder:\n%s", r.UserID, body),
			AllowedMentions: onlyOwner,
		})
	}
	return err
}
