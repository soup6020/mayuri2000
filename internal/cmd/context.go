package cmd

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// Context carries everything a command needs, independent of whether it was
// invoked through a slash command, a context menu or a text prefix.
type Context struct {
	context.Context
	Client   *bot.Client
	Registry *Registry

	CommandName string
	// Prefix is the prefix the command was invoked with, or "/" for
	// interactions.
	Prefix    string
	GuildID   *snowflake.ID
	ChannelID snowflake.ID
	Author    discord.User
	// Member is the invoking member; nil outside guilds.
	Member *discord.Member
	// Permissions of the invoking member (0 outside guilds).
	Permissions discord.Permissions

	// SourceMessage is the invoking message for prefix commands.
	SourceMessage *discord.Message
	// TargetMessage is set for message context menu commands.
	TargetMessage *discord.Message
	// TargetUser is set for user context menu commands.
	TargetUser *discord.User
	// Invocation records how the command was invoked.
	Invocation Invocation
	// Extras holds prefix tokens that were not assigned to any option.
	Extras []string

	attachmentSizeLimit int
	args                map[string]any
	resp                responder
}

func (c *Context) IsInteraction() bool { return c.SourceMessage == nil }

// FetchMember returns a member of the current guild from cache or REST.
func (c *Context) FetchMember(userID snowflake.ID) (*discord.Member, error) {
	if c.GuildID == nil {
		return nil, Errorf("This only works in a server.")
	}
	return c.Registry.member(c.Client, *c.GuildID, userID)
}

func (c *Context) Has(name string) bool { _, ok := c.args[name]; return ok }

func (c *Context) OptString(name string) (string, bool) { v, ok := c.args[name].(string); return v, ok }
func (c *Context) OptInt(name string) (int, bool)       { v, ok := c.args[name].(int); return v, ok }
func (c *Context) OptBool(name string) (bool, bool)     { v, ok := c.args[name].(bool); return v, ok }
func (c *Context) OptFloat(name string) (float64, bool) {
	v, ok := c.args[name].(float64)
	return v, ok
}
func (c *Context) OptUser(name string) (discord.User, bool) {
	v, ok := c.args[name].(discord.User)
	return v, ok
}
func (c *Context) OptAttachment(name string) (discord.Attachment, bool) {
	v, ok := c.args[name].(discord.Attachment)
	return v, ok
}

func (c *Context) String(name string) string { v, _ := c.OptString(name); return v }
func (c *Context) Int(name string) int       { v, _ := c.OptInt(name); return v }
func (c *Context) Bool(name string) bool     { v, _ := c.OptBool(name); return v }
func (c *Context) Float(name string) float64 { v, _ := c.OptFloat(name); return v }

func (c *Context) StringOr(name, def string) string {
	if v, ok := c.OptString(name); ok {
		return v
	}
	return def
}

func (c *Context) IntOr(name string, def int) int {
	if v, ok := c.OptInt(name); ok {
		return v
	}
	return def
}

func (c *Context) FloatOr(name string, def float64) float64 {
	if v, ok := c.OptFloat(name); ok {
		return v
	}
	return def
}

// UploadLimit returns the maximum attachment size in bytes the bot may upload
// in the current channel.
func (c *Context) UploadLimit() int {
	if c.attachmentSizeLimit > 0 {
		return c.attachmentSizeLimit
	}
	const mib = 1 << 20
	if c.GuildID != nil {
		if g, ok := c.Client.Caches.Guild(*c.GuildID); ok {
			switch g.PremiumTier {
			case discord.PremiumTier2:
				return 50 * mib
			case discord.PremiumTier3:
				return 100 * mib
			}
		}
	}
	return 10 * mib
}

// Defer acknowledges the command, showing a loading state. For prefix
// commands this starts the typing indicator.
func (c *Context) Defer() error { return c.resp.deferReply(false) }

// Send sends a message in response to the command.
func (c *Context) Send(msg discord.MessageCreate) error { return c.resp.send(msg) }

func (c *Context) Reply(content string) error {
	return c.Send(discord.MessageCreate{Content: content})
}

// ReplyEphemeral sends a message that only the invoker can see where
// possible (interactions that were not deferred publicly).
func (c *Context) ReplyEphemeral(content string) error {
	return c.Send(discord.MessageCreate{Content: content, Flags: discord.MessageFlagEphemeral})
}

func (c *Context) ReplyEmbeds(embeds ...discord.Embed) error {
	return c.Send(discord.MessageCreate{Embeds: embeds})
}

// ReplyFile uploads data as a file, checking the channel's upload limit first.
func (c *Context) ReplyFile(content, name string, data []byte) error {
	if len(data) > c.UploadLimit() {
		return Errorf("The result is too large to upload (%.1f MiB).", float64(len(data))/(1<<20))
	}
	return c.Send(discord.MessageCreate{
		Content: content,
		Files:   []*discord.File{discord.NewFile(name, "", bytes.NewReader(data))},
	})
}

type responder interface {
	deferReply(ephemeral bool) error
	send(msg discord.MessageCreate) error
	done()
}

var noMentions = &discord.AllowedMentions{RepliedUser: false}

// interactionResponder answers application command interactions.
type interactionResponder struct {
	e         *events.ApplicationCommandInteractionCreate
	mu        sync.Mutex
	deferred  bool
	responded bool
}

func (r *interactionResponder) deferReply(ephemeral bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deferred || r.responded {
		return nil
	}
	r.deferred = true
	return r.e.DeferCreateMessage(ephemeral)
}

func (r *interactionResponder) send(msg discord.MessageCreate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg.AllowedMentions == nil {
		msg.AllowedMentions = noMentions
	}
	switch {
	case !r.deferred && !r.responded:
		// Only count as responded on success, so an error reply can still
		// use the initial response instead of an invalid followup.
		err := r.e.CreateMessage(msg)
		r.responded = err == nil
		return err
	case r.deferred && !r.responded:
		update := discord.MessageUpdate{
			Content:         &msg.Content,
			Files:           msg.Files,
			AllowedMentions: msg.AllowedMentions,
		}
		if len(msg.Embeds) > 0 {
			update.Embeds = &msg.Embeds
		}
		if len(msg.Components) > 0 {
			update.Components = &msg.Components
		}
		_, err := r.e.Client().Rest.UpdateInteractionResponse(r.e.ApplicationID(), r.e.Token(), update)
		r.responded = err == nil
		return err
	default:
		_, err := r.e.Client().Rest.CreateFollowupMessage(r.e.ApplicationID(), r.e.Token(), msg)
		return err
	}
}

func (r *interactionResponder) done() {}

// messageResponder answers prefix commands by replying to the source message.
type messageResponder struct {
	client    *bot.Client
	channelID snowflake.ID
	messageID snowflake.ID

	mu         sync.Mutex
	stopTyping context.CancelFunc
}

func (r *messageResponder) deferReply(bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopTyping != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	r.stopTyping = cancel
	go func() {
		t := time.NewTicker(8 * time.Second)
		defer t.Stop()
		for {
			_ = r.client.Rest.SendTyping(r.channelID)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return nil
}

func (r *messageResponder) send(msg discord.MessageCreate) error {
	r.done()
	msg.Flags = msg.Flags.Remove(discord.MessageFlagEphemeral)
	if msg.AllowedMentions == nil {
		msg.AllowedMentions = noMentions
	}
	msg.MessageReference = &discord.MessageReference{
		MessageID:       &r.messageID,
		ChannelID:       &r.channelID,
		FailIfNotExists: false,
	}
	_, err := r.client.Rest.CreateMessage(r.channelID, msg)
	return err
}

func (r *messageResponder) done() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopTyping != nil {
		r.stopTyping()
	}
}
