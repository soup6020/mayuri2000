// Package reminders provides /remind and /reminders, and the Scheduler that
// delivers them.
package reminders

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/store"
	"mayuri2000/internal/when"
)

const (
	MaxAhead        = 21 * 24 * time.Hour
	minAhead        = 10 * time.Second
	maxPerUser      = 25
	maxMessageRunes = 1000
)

type Module struct {
	scheduler *Scheduler
}

func New(s *Scheduler) *Module { return &Module{scheduler: s} }

func (m *Module) Commands() []*cmd.Command {
	return []*cmd.Command{
		{
			Name:        "remind",
			Aliases:     []string{"remindme", "reminder"},
			Category:    "Reminders",
			Description: "Get reminded by DM or in a channel after a delay or at a time",
			Options: []cmd.Option{
				// Channel comes first so prefix use reads "%remind [#channel] <when> <message>".
				{Name: "channel", Description: "Post the reminder in this channel instead of DMing you", Type: cmd.Channel},
				{Name: "when", Description: "A delay like 3h12m or a time like 20:15, 8pm, tomorrow 9am (max 21 days)", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(64)},
				{Name: "message", Description: "What to remind you about", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(maxMessageRunes)},
			},
			Run: m.remind,
		},
		{
			Name:        "reminders",
			Category:    "Reminders",
			Description: "List or cancel your reminders",
			Subcommands: []*cmd.Command{
				{Name: "list", Description: "List your pending reminders", Run: m.list},
				{
					Name: "cancel", Aliases: []string{"delete", "remove"}, Description: "Cancel a reminder by its ID",
					Options: []cmd.Option{{Name: "id", Description: "Reminder ID from /reminders list", Type: cmd.Int, Required: true, Min: cmd.Ptr(1)}},
					Run:     m.cancel,
				},
			},
		},
	}
}

// UserLocation returns the user's saved time zone, or UTC.
func UserLocation(c *cmd.Context) (*time.Location, bool) {
	name, ok, err := c.Registry.Store().UserTimezone(c, c.Author.ID)
	if err != nil || !ok {
		return time.UTC, false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, false
	}
	return loc, true
}

// isDelay reports whether the time spec is a relative delay rather than a
// clock time (which depends on the user's time zone).
func isDelay(spec string) bool {
	_, err := when.ParseDuration(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(spec)), "in "))
	return err == nil
}

func (m *Module) remind(c *cmd.Context) error {
	now := time.Now()
	loc, hasZone := UserLocation(c)

	var due time.Time
	var message, spec string
	var err error
	if c.IsInteraction() {
		spec, message = c.String("when"), strings.TrimSpace(c.String("message"))
		due, err = when.Parse(spec, now, loc)
	} else {
		// Prefix: "%remind tomorrow 9am take out the trash" arrives split at
		// the first space, so find where the time ends ourselves.
		text := strings.TrimSpace(c.String("when") + " " + c.String("message"))
		due, message, err = when.Split(text, now, loc)
		spec = strings.TrimSpace(strings.TrimSuffix(text, message))
	}
	if err != nil {
		return cmd.Errorf("I don't understand that time (%s). Try `3h12m`, `20:15`, `8:30pm`, `tomorrow 9am` or `friday 18:00`.", err)
	}
	if message == "" {
		return cmd.Errorf("What should I remind you about?")
	}
	if len([]rune(message)) > maxMessageRunes {
		return cmd.Errorf("Reminders can be at most %d characters.", maxMessageRunes)
	}
	switch ahead := due.Sub(now); {
	case ahead < minAhead:
		return cmd.Errorf("That's too soon; reminders need to be at least 10 seconds away.")
	case ahead > MaxAhead:
		return cmd.Errorf("Reminders can be at most 21 days away (that's <t:%d:f>).", due.Unix())
	}

	target, hasTarget := c.OptChannel("channel")
	if hasTarget {
		if err := checkChannel(c, target); err != nil {
			return err
		}
	}

	st := c.Registry.Store()
	existing, err := st.UserReminders(c, c.Author.ID)
	if err != nil {
		return err
	}
	if len(existing) >= maxPerUser {
		return cmd.Errorf("You already have %d reminders, the maximum. Cancel some with `/reminders cancel`.", maxPerUser)
	}

	r := store.Reminder{
		UserID:          c.Author.ID,
		ChannelID:       target,
		SourceChannelID: c.ChannelID,
		Message:         message,
		DueAt:           due,
		CreatedAt:       now,
	}
	if c.GuildID != nil {
		r.GuildID = *c.GuildID
	}
	if r, err = st.AddReminder(c, r); err != nil {
		return err
	}
	m.scheduler.Wake()

	where := "by DM"
	if hasTarget {
		where = fmt.Sprintf("in <#%s>", target)
	}
	reply := fmt.Sprintf("⏰ Okay, I'll remind you %s <t:%d:R> (<t:%d:F>).\n-# Reminder #%d · cancel with `/reminders cancel id:%d`",
		where, due.Unix(), due.Unix(), r.ID, r.ID)
	if !hasZone && !isDelay(spec) {
		reply += "\n-# That time was read as UTC. Set your time zone with `/timezone` so times like 20:15 mean your local time."
	}
	return c.Reply(reply)
}

// checkChannel verifies the target is a channel in this server that both the
// user and the bot can send messages in.
func checkChannel(c *cmd.Context, id snowflake.ID) error {
	if c.GuildID == nil {
		return cmd.Errorf("Channel reminders only work in servers. Leave out the channel to get a DM.")
	}
	ch, ok := c.Client.Caches.Channel(id)
	if !ok {
		return cmd.Errorf("I can't see that channel.")
	}
	if ch.GuildID() != *c.GuildID {
		return cmd.Errorf("That channel isn't in this server.")
	}
	if !slices.Contains(cmd.TextChannelTypes, ch.Type()) {
		return cmd.Errorf("I can only post reminders in text channels.")
	}
	need := discord.PermissionViewChannel | discord.PermissionSendMessages
	if c.Member != nil && !c.Client.Caches.MemberPermissionsInChannel(ch, *c.Member).Has(need) {
		return cmd.Errorf("You can't send messages in <#%s>, so I won't post reminders there for you.", id)
	}
	if self, ok := c.Client.Caches.SelfMember(*c.GuildID); ok && !c.Client.Caches.MemberPermissionsInChannel(ch, self).Has(need) {
		return cmd.Errorf("I can't send messages in <#%s>.", id)
	}
	return nil
}

func (m *Module) list(c *cmd.Context) error {
	rs, err := c.Registry.Store().UserReminders(c, c.Author.ID)
	if err != nil {
		return err
	}
	if len(rs) == 0 {
		return c.ReplyEphemeral("You have no reminders. Set one with `/remind`.")
	}
	var lines []string
	for _, r := range rs {
		where := "DM"
		if r.ChannelID != 0 {
			where = fmt.Sprintf("<#%s>", r.ChannelID)
		}
		text := strings.ReplaceAll(r.Message, "\n", " ")
		if rs := []rune(text); len(rs) > 80 {
			text = string(rs[:79]) + "…"
		}
		lines = append(lines, fmt.Sprintf("`#%d` <t:%d:R> · %s · %s", r.ID, r.DueAt.Unix(), where, text))
	}
	var pages []cmd.Page
	for i := 0; i < len(lines); i += 10 {
		pages = append(pages, cmd.Page{Embed: &discord.Embed{
			Title:       fmt.Sprintf("Your reminders (%d)", len(lines)),
			Description: strings.Join(lines[i:min(i+10, len(lines))], "\n"),
			Footer:      &discord.EmbedFooter{Text: "Cancel one with /reminders cancel id:<id>"},
			Color:       0xF1C40F,
		}})
	}
	return c.Paginate(pages)
}

func (m *Module) cancel(c *cmd.Context) error {
	id := int64(c.Int("id"))
	err := c.Registry.Store().DeleteReminder(c, id, c.Author.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return cmd.Errorf("You don't have a reminder #%d.", id)
	}
	if err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("Cancelled reminder #%d.", id))
}
