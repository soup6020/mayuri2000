package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/store"
)

const heavyTimeout = 3 * time.Minute

// Registry dispatches slash commands, context menus, components and prefix
// messages to the registered commands.
type Registry struct {
	log           *slog.Logger
	store         *store.Store
	defaultPrefix string

	commands []*Command
	byName   map[string]*Command
	menus    map[string]*Menu

	components map[string]ComponentHandler

	guildLocks sync.Map // snowflake.ID -> chan struct{}
	workers    chan struct{}

	stats *Stats
}

// ComponentHandler handles a component interaction whose custom ID begins
// with the registered prefix followed by ':'. data is the remainder.
type ComponentHandler func(e *events.ComponentInteractionCreate, data string) error

func NewRegistry(log *slog.Logger, st *store.Store, defaultPrefix string) *Registry {
	r := &Registry{
		log:           log,
		store:         st,
		defaultPrefix: defaultPrefix,
		byName:        map[string]*Command{},
		menus:         map[string]*Menu{},
		components:    map[string]ComponentHandler{},
		workers:       make(chan struct{}, max(2, runtime.NumCPU())),
		stats:         newStats(),
	}
	r.HandleComponent("pg", handlePageButton)
	return r
}

func (r *Registry) Add(cmds ...*Command) {
	for _, c := range cmds {
		for _, name := range append([]string{c.Name}, c.Aliases...) {
			if _, dup := r.byName[name]; dup {
				panic("duplicate command name " + name)
			}
			r.byName[name] = c
		}
		for _, s := range c.Subcommands {
			s.parent = c
			if s.Category == "" {
				s.Category = c.Category
			}
		}
		r.commands = append(r.commands, c)
	}
}

func (r *Registry) AddMenus(menus ...*Menu) {
	for _, m := range menus {
		r.menus[m.Name] = m
	}
}

func (r *Registry) HandleComponent(prefix string, h ComponentHandler) {
	r.components[prefix] = h
}

// Commands returns all registered commands sorted by category then name.
func (r *Registry) Commands() []*Command {
	out := append([]*Command(nil), r.commands...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) Lookup(name string) (*Command, bool) {
	c, ok := r.byName[strings.ToLower(name)]
	return c, ok
}

// Prefix returns the prefix in effect for the guild (nil for DMs).
func (r *Registry) Prefix(guildID *snowflake.ID) string {
	if guildID != nil {
		if p, ok := r.store.Prefix(*guildID); ok {
			return p
		}
	}
	return r.defaultPrefix
}

func (r *Registry) DefaultPrefix() string { return r.defaultPrefix }
func (r *Registry) Stats() *Stats         { return r.stats }
func (r *Registry) Store() *store.Store   { return r.store }

// Sync registers all application commands with Discord.
func (r *Registry) Sync(client *bot.Client, guildIDs []snowflake.ID) error {
	var creates []discord.ApplicationCommandCreate
	for _, c := range r.commands {
		creates = append(creates, c.slashCreate())
	}
	for _, m := range r.menus {
		creates = append(creates, m.create())
	}
	return handler.SyncCommands(client, creates, guildIDs)
}

// OnEvent implements bot.EventListener.
func (r *Registry) OnEvent(event bot.Event) {
	switch e := event.(type) {
	case *events.ApplicationCommandInteractionCreate:
		go r.onApplicationCommand(e)
	case *events.ComponentInteractionCreate:
		go r.onComponent(e)
	case *events.AutocompleteInteractionCreate:
		go r.onAutocomplete(e)
	case *events.MessageCreate:
		go r.onMessage(e)
	}
}

func (r *Registry) onApplicationCommand(e *events.ApplicationCommandInteractionCreate) {
	c := &Context{
		Context:             context.Background(),
		Client:              e.Client(),
		Registry:            r,
		Prefix:              "/",
		GuildID:             e.GuildID(),
		ChannelID:           e.Channel().ID(),
		Author:              e.User(),
		attachmentSizeLimit: e.AttachmentSizeLimit(),
		args:                map[string]any{},
		resp:                &interactionResponder{e: e},
	}
	if m := e.Member(); m != nil {
		member := m.Member
		if gid := e.GuildID(); gid != nil {
			member.GuildID = *gid
		}
		c.Member = &member
		c.Permissions = m.Permissions
	}

	switch data := e.Data.(type) {
	case discord.SlashCommandInteractionData:
		cmd, ok := r.byName[data.CommandName()]
		if !ok {
			return
		}
		if data.SubCommandName != nil {
			if cmd, ok = cmd.Sub(*data.SubCommandName); !ok {
				return
			}
		}
		c.CommandName = cmd.FullName()
		for _, opt := range cmd.Options {
			switch opt.Type {
			case String:
				if v, ok := data.OptString(opt.Name); ok {
					c.args[opt.Name] = v
				}
			case Int:
				if v, ok := data.OptInt(opt.Name); ok {
					c.args[opt.Name] = v
				}
			case Float:
				if v, ok := data.OptFloat(opt.Name); ok {
					c.args[opt.Name] = v
				}
			case Bool:
				if v, ok := data.OptBool(opt.Name); ok {
					c.args[opt.Name] = v
				}
			case User:
				if v, ok := data.OptUser(opt.Name); ok {
					c.args[opt.Name] = v
				}
			case Attachment:
				if v, ok := data.OptAttachment(opt.Name); ok {
					c.args[opt.Name] = v
				}
			case Role:
				if v, ok := data.OptRole(opt.Name); ok {
					if gid := e.GuildID(); gid != nil {
						v.GuildID = *gid
					}
					c.args[opt.Name] = v
				}
			case Channel:
				if v, ok := data.OptChannel(opt.Name); ok {
					c.args[opt.Name] = v.ID
				}
			}
		}
		perms, guildOnly, heavy := cmd.effective()
		r.run(c, heavy, perms, guildOnly, cmd.Run)

	case discord.UserCommandInteractionData:
		m, ok := r.menus[data.CommandName()]
		if !ok {
			return
		}
		c.CommandName = m.Name
		c.Invocation = InvokedMenu
		u := data.TargetUser()
		c.TargetUser = &u
		r.run(c, m.Heavy, 0, false, m.Run)

	case discord.MessageCommandInteractionData:
		m, ok := r.menus[data.CommandName()]
		if !ok {
			return
		}
		c.CommandName = m.Name
		c.Invocation = InvokedMenu
		msg := data.TargetMessage()
		c.TargetMessage = &msg
		r.run(c, m.Heavy, 0, false, m.Run)
	}
}

func (r *Registry) onAutocomplete(e *events.AutocompleteInteractionCreate) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("panic in autocomplete", "panic", p, "stack", string(debug.Stack()))
		}
	}()
	cmd, ok := r.byName[e.Data.CommandName]
	if !ok {
		return
	}
	if e.Data.SubCommandName != nil {
		if cmd, ok = cmd.Sub(*e.Data.SubCommandName); !ok {
			return
		}
	}
	focused := e.Data.Focused()
	var input string
	_ = json.Unmarshal(focused.Value, &input)
	var choices []discord.AutocompleteChoice
	for _, o := range cmd.Options {
		if o.Name != focused.Name || o.Autocomplete == nil {
			continue
		}
		for _, ch := range o.Autocomplete(input) {
			if len(choices) == 25 {
				break
			}
			choices = append(choices, discord.AutocompleteChoiceString{Name: truncateRunes(ch.Name, 100), Value: truncateRunes(ch.Value, 100)})
		}
	}
	if err := e.AutocompleteResult(choices); err != nil {
		r.log.Debug("autocomplete response failed", "command", cmd.Name, "err", err)
	}
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (r *Registry) onComponent(e *events.ComponentInteractionCreate) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("panic in component handler", "panic", p, "stack", string(debug.Stack()))
		}
	}()
	id := e.Data.CustomID()
	prefix, data, _ := strings.Cut(id, ":")
	h, ok := r.components[prefix]
	if !ok {
		return
	}
	if err := h(e, data); err != nil {
		r.log.Error("component handler failed", "custom_id", id, "err", err)
	}
}

func (r *Registry) onMessage(e *events.MessageCreate) {
	msg := e.Message
	if msg.Author.Bot || msg.Author.System || msg.WebhookID != nil {
		return
	}
	rest, prefix, ok := r.stripPrefix(e.Client(), msg)
	if !ok {
		return
	}
	cmd, argText, ok := r.Resolve(msg.GuildID, rest)
	if !ok {
		return
	}
	if len(cmd.Subcommands) > 0 {
		var subName string
		subName, argText = splitCommand(argText)
		sub, found := cmd.Sub(subName)
		if !found {
			_, _ = e.Client().Rest.CreateMessage(msg.ChannelID, discord.MessageCreate{
				Content:          GroupUsage(prefix, cmd),
				AllowedMentions:  noMentions,
				MessageReference: &discord.MessageReference{MessageID: &msg.ID, FailIfNotExists: false},
			})
			return
		}
		cmd = sub
	}

	resp := &messageResponder{client: e.Client(), channelID: msg.ChannelID, messageID: msg.ID}
	c := &Context{
		Context:       context.Background(),
		Client:        e.Client(),
		Registry:      r,
		CommandName:   cmd.FullName(),
		Prefix:        prefix,
		GuildID:       msg.GuildID,
		ChannelID:     msg.ChannelID,
		Author:        msg.Author,
		SourceMessage: &msg,
		Invocation:    InvokedPrefix,
		args:          map[string]any{},
		resp:          resp,
	}

	if msg.GuildID != nil {
		member, err := r.member(e.Client(), *msg.GuildID, msg.Author.ID)
		if err != nil {
			r.log.Warn("could not resolve member", "guild", *msg.GuildID, "user", msg.Author.ID, "err", err)
		} else {
			c.Member = member
			c.Permissions = e.Client().Caches.MemberPermissions(*member)
		}
	}

	args, missing, extras := assign(cmd.Options, strings.TrimSpace(argText))
	if len(missing) > 0 {
		_ = c.Reply(fmt.Sprintf("Missing required argument `%s`.\nUsage: `%s`", missing[0].Name, Usage(prefix, cmd)))
		return
	}
	c.Extras = extras
	for _, a := range args {
		v, err := r.convert(c, a)
		if err != nil {
			_ = c.Reply(fmt.Sprintf("Invalid value for `%s`: %s\nUsage: `%s`", a.opt.Name, err, Usage(prefix, cmd)))
			return
		}
		c.args[a.opt.Name] = v
	}
	// Attachment options are filled from the message's attachments in order.
	atts := msg.Attachments
	for _, o := range cmd.Options {
		if o.Type == Attachment && len(atts) > 0 {
			c.args[o.Name] = atts[0]
			atts = atts[1:]
		}
	}
	perms, guildOnly, heavy := cmd.effective()
	r.run(c, heavy, perms, guildOnly, cmd.Run)
}

// Resolve finds the command for prefix-stripped input, expanding guild
// aliases. An alias's stored arguments come before the invocation's own.
func (r *Registry) Resolve(guildID *snowflake.ID, input string) (*Command, string, bool) {
	name, argText := splitCommand(input)
	if cmd, ok := r.Lookup(name); ok {
		return cmd, argText, true
	}
	if guildID == nil {
		return nil, "", false
	}
	alias, ok := r.store.Alias(*guildID, strings.ToLower(name))
	if !ok {
		return nil, "", false
	}
	target, aliasArgs := splitCommand(alias.Command)
	cmd, ok := r.Lookup(target)
	if !ok {
		return nil, "", false
	}
	return cmd, strings.TrimSpace(aliasArgs + " " + argText), true
}

// splitCommand splits "name args..." into the command name and the rest.
func splitCommand(s string) (name, args string) {
	s = strings.TrimLeft(s, " \t\n")
	i := strings.IndexAny(s, " \t\n")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func (r *Registry) stripPrefix(client *bot.Client, msg discord.Message) (string, string, bool) {
	prefix := r.Prefix(msg.GuildID)
	if strings.HasPrefix(msg.Content, prefix) {
		return msg.Content[len(prefix):], prefix, true
	}
	self := client.ID()
	for _, m := range []string{"<@" + self.String() + ">", "<@!" + self.String() + ">"} {
		if strings.HasPrefix(msg.Content, m) {
			return msg.Content[len(m):], prefix, true
		}
	}
	return "", "", false
}

func (r *Registry) member(client *bot.Client, guildID, userID snowflake.ID) (*discord.Member, error) {
	if m, ok := client.Caches.Member(guildID, userID); ok {
		return &m, nil
	}
	m, err := client.Rest.GetMember(guildID, userID)
	if err != nil {
		return nil, err
	}
	m.GuildID = guildID
	return m, nil
}

func (r *Registry) convert(c *Context, a rawArg) (any, error) {
	switch a.opt.Type {
	case Int:
		n, err := strconv.Atoi(a.value)
		if err != nil {
			return nil, fmt.Errorf("`%s` is not a whole number", a.value)
		}
		if a.opt.Min != nil && n < *a.opt.Min {
			return nil, fmt.Errorf("must be at least %d", *a.opt.Min)
		}
		if a.opt.Max != nil && n > *a.opt.Max {
			return nil, fmt.Errorf("must be at most %d", *a.opt.Max)
		}
		return n, nil
	case Float:
		f, err := strconv.ParseFloat(a.value, 64)
		if err != nil {
			return nil, fmt.Errorf("`%s` is not a number", a.value)
		}
		return f, nil
	case Bool:
		b, ok := parseBool(a.value)
		if !ok {
			return nil, fmt.Errorf("`%s` is not yes/no", a.value)
		}
		return b, nil
	case User:
		id, ok := ParseUserID(a.value)
		if !ok {
			return nil, fmt.Errorf("`%s` is not a user mention or ID", a.value)
		}
		return r.user(c.Client, c.GuildID, id)
	case Channel:
		if c.GuildID == nil {
			return nil, fmt.Errorf("channels only exist in servers")
		}
		var channels []discord.GuildChannel
		for ch := range c.Client.Caches.ChannelsForGuild(*c.GuildID) {
			if slices.Contains(TextChannelTypes, ch.Type()) {
				channels = append(channels, ch)
			}
		}
		return FindChannel(channels, a.value)
	case Role:
		if c.GuildID == nil {
			return nil, fmt.Errorf("roles only exist in servers")
		}
		var roles []discord.Role
		for role := range c.Client.Caches.Roles(*c.GuildID) {
			roles = append(roles, role)
		}
		return FindRole(roles, a.value)
	default:
		if a.opt.MaxLength != nil && len([]rune(a.value)) > *a.opt.MaxLength {
			return nil, fmt.Errorf("must be at most %d characters", *a.opt.MaxLength)
		}
		if len(a.opt.Choices) > 0 {
			for _, ch := range a.opt.Choices {
				if strings.EqualFold(ch.Value, a.value) || strings.EqualFold(ch.Name, a.value) {
					return ch.Value, nil
				}
			}
			names := make([]string, len(a.opt.Choices))
			for i, ch := range a.opt.Choices {
				names[i] = ch.Value
			}
			return nil, fmt.Errorf("must be one of %s", strings.Join(names, ", "))
		}
		return a.value, nil
	}
}

func (r *Registry) user(client *bot.Client, guildID *snowflake.ID, id snowflake.ID) (discord.User, error) {
	if guildID != nil {
		if m, ok := client.Caches.Member(*guildID, id); ok {
			return m.User, nil
		}
	}
	u, err := client.Rest.GetUser(id)
	if err != nil {
		return discord.User{}, fmt.Errorf("unknown user `%s`", id)
	}
	return *u, nil
}

func (r *Registry) run(c *Context, heavy bool, perms discord.Permissions, guildOnly bool, fn func(*Context) error) {
	log := r.log.With("command", c.CommandName, "user", c.Author.ID, "guild", c.GuildID)
	defer c.resp.done()
	defer func() {
		if p := recover(); p != nil {
			log.Error("panic in command", "panic", p, "stack", string(debug.Stack()))
			_ = c.ReplyEphemeral("Something went wrong running that command.")
		}
	}()

	if guildOnly && c.GuildID == nil {
		_ = c.ReplyEphemeral("This command can only be used in a server.")
		return
	}
	if perms != 0 && !c.Permissions.Has(perms) {
		_ = c.ReplyEphemeral(fmt.Sprintf("You need the %s permission(s) to use this.", perms.String()))
		return
	}

	if heavy {
		if err := c.Defer(); err != nil {
			log.Error("failed to defer", "err", err)
			return
		}
		ctx, cancel := context.WithTimeout(c.Context, heavyTimeout)
		defer cancel()
		c.Context = ctx
		release, err := r.acquire(ctx, c.GuildID)
		if err != nil {
			_ = c.Reply("Too many image jobs are queued right now, try again in a bit.")
			return
		}
		defer release()
	}

	start := time.Now()
	err := fn(c)
	log.Debug("command finished", "took", time.Since(start), "err", err)
	_, isUserErr := userMessage(err)
	r.stats.record(c.CommandName, c.Invocation, err != nil && !isUserErr)
	if heavy {
		r.stats.ImageJobs.Add(1)
	}
	if err := r.store.IncrementCommand(context.Background(), c.CommandName); err != nil {
		log.Warn("failed to record command stats", "err", err)
	}
	if err == nil {
		return
	}
	if msg, ok := userMessage(err); ok {
		_ = c.ReplyEphemeral(msg)
		return
	}
	log.Error("command failed", "err", err)
	_ = c.ReplyEphemeral("Something went wrong running that command.")
}

// acquire serialises heavy commands per guild and bounds them globally.
func (r *Registry) acquire(ctx context.Context, guildID *snowflake.ID) (func(), error) {
	var guildLock chan struct{}
	if guildID != nil {
		l, _ := r.guildLocks.LoadOrStore(*guildID, make(chan struct{}, 1))
		guildLock = l.(chan struct{})
		select {
		case guildLock <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case r.workers <- struct{}{}:
	case <-ctx.Done():
		if guildLock != nil {
			<-guildLock
		}
		return nil, ctx.Err()
	}
	return func() {
		<-r.workers
		if guildLock != nil {
			<-guildLock
		}
	}, nil
}

// GroupUsage lists a command group's subcommands for prefix invocation.
func GroupUsage(prefix string, c *Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", c.Description)
	for _, s := range c.Subcommands {
		fmt.Fprintf(&b, "`%s` %s\n", Usage(prefix, s), s.Description)
	}
	return strings.TrimSpace(b.String())
}

// Usage renders a one-line usage string for prefix invocation.
func Usage(prefix string, c *Command) string {
	if len(c.Subcommands) > 0 {
		return prefix + c.Name + " <" + subNames(c) + "> ..."
	}
	var b strings.Builder
	b.WriteString(prefix + c.FullName())
	for _, o := range c.Options {
		if o.SlashOnly || o.Type == Attachment {
			continue
		}
		name := o.Name
		if o.Rest {
			name += "..."
		}
		if o.Required {
			b.WriteString(" <" + name + ">")
		} else {
			b.WriteString(" [" + name + "]")
		}
	}
	return b.String()
}

func subNames(c *Command) string {
	names := make([]string, len(c.Subcommands))
	for i, s := range c.Subcommands {
		names[i] = s.Name
	}
	return strings.Join(names, "|")
}
