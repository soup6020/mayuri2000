// Package feeds provides the /rss commands for subscribing channels to
// RSS/Atom feeds. Polling happens in internal/rss.Poller.
package feeds

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/rss"
	"mayuri2000/internal/safehttp"
	"mayuri2000/internal/store"
)

const maxFeedsPerGuild = 50

var feedNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

type Module struct {
	fetcher *rss.Fetcher
}

func New(fetcher *rss.Fetcher) *Module { return &Module{fetcher: fetcher} }

func nameOpt() cmd.Option {
	return cmd.Option{Name: "name", Description: "Feed name", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(32)}
}

func channelOpt(desc string) cmd.Option {
	return cmd.Option{Name: "channel", Description: desc, Type: cmd.Channel}
}

func (m *Module) Commands() []*cmd.Command {
	return []*cmd.Command{{
		Name:        "rss",
		Aliases:     []string{"feed", "feeds"},
		Category:    "Feeds",
		Description: "Subscribe channels to RSS/Atom feeds",
		GuildOnly:   true,
		Perms:       discord.PermissionManageChannels,
		Subcommands: []*cmd.Command{
			{
				Name: "add", Description: "Subscribe a channel to a feed",
				Options: []cmd.Option{
					{Name: "name", Description: "Short name for the feed (letters, digits, - and _)", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(32)},
					{Name: "url", Description: "Feed URL (use /rss find to discover one from a website)", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(500)},
					channelOpt("Channel to post in (default: this channel)"),
				},
				Run: m.add,
			},
			{
				Name: "remove", Aliases: []string{"delete", "del"}, Description: "Unsubscribe from a feed",
				Options: []cmd.Option{nameOpt(), channelOpt("Channel the feed posts in (default: this channel)")},
				Run:     m.remove,
			},
			{
				Name: "list", Description: "List this server's feeds",
				Options: []cmd.Option{channelOpt("Only show feeds in this channel")},
				Run:     m.list,
			},
			{
				Name: "template", Description: "Show or change how a feed's posts look",
				Options: []cmd.Option{
					nameOpt(),
					channelOpt("Channel the feed posts in (default: this channel)"),
					{Name: "template", Description: `Text with variables, e.g. "$title\n$link"; "reset" for the default`, Type: cmd.String, Rest: true, MaxLength: cmd.Ptr(1000)},
				},
				Run: m.template,
			},
			{
				Name: "embed", Description: "Post a feed's items as rich embeds instead of text",
				Options: []cmd.Option{
					nameOpt(),
					{Name: "enabled", Description: "Use embeds", Type: cmd.Bool, Required: true},
					channelOpt("Channel the feed posts in (default: this channel)"),
				},
				Run: m.embed,
			},
			{
				Name: "force", Description: "Post a feed's latest item now",
				Options: []cmd.Option{nameOpt(), channelOpt("Channel the feed posts in (default: this channel)")},
				Run:     m.force,
			},
			{
				Name: "find", Description: "Find the feeds a website offers",
				Options: []cmd.Option{{Name: "url", Description: "Website URL", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(500)}},
				Run:     m.find,
			},
		},
	}}
}

// targetChannel returns the channel option or the current channel.
func targetChannel(c *cmd.Context) snowflake.ID {
	if ch, ok := c.OptChannel("channel"); ok {
		return ch
	}
	return c.ChannelID
}

// resolveFeed finds a feed by name: in the given channel, else the current
// channel, else anywhere in the server if the name is unique.
func resolveFeed(c *cmd.Context) (store.Feed, error) {
	name := strings.ToLower(c.String("name"))
	feeds, err := c.Registry.Store().GuildFeeds(c, *c.GuildID)
	if err != nil {
		return store.Feed{}, err
	}
	explicit, hasChannel := c.OptChannel("channel")
	var matches []store.Feed
	for _, f := range feeds {
		if f.Name != name {
			continue
		}
		if hasChannel && f.ChannelID != explicit {
			continue
		}
		if !hasChannel && f.ChannelID == c.ChannelID {
			return f, nil
		}
		matches = append(matches, f)
	}
	switch len(matches) {
	case 0:
		where := ""
		if hasChannel {
			where = fmt.Sprintf(" in <#%s>", explicit)
		}
		return store.Feed{}, cmd.Errorf("There's no feed named `%s`%s. See `/rss list`.", name, where)
	case 1:
		return matches[0], nil
	}
	return store.Feed{}, cmd.Errorf("Several channels have a feed named `%s`; pick one with the `channel` option.", name)
}

// checkCanPost verifies the bot can send (and embed, if needed) in a channel.
func checkCanPost(c *cmd.Context, channelID snowflake.ID, embed bool) error {
	ch, ok := c.Client.Caches.Channel(channelID)
	if !ok {
		return nil // threads and uncached channels: find out when posting
	}
	if ch.GuildID() != *c.GuildID {
		return cmd.Errorf("That channel isn't in this server.")
	}
	self, ok := c.Client.Caches.SelfMember(*c.GuildID)
	if !ok {
		return nil
	}
	need := discord.PermissionViewChannel | discord.PermissionSendMessages
	if embed {
		need |= discord.PermissionEmbedLinks
	}
	if !c.Client.Caches.MemberPermissionsInChannel(ch, self).Has(need) {
		perms := "View Channel and Send Messages"
		if embed {
			perms = "View Channel, Send Messages and Embed Links"
		}
		return cmd.Errorf("I can't post in <#%s>. I need %s there.", channelID, perms)
	}
	return nil
}

func (m *Module) add(c *cmd.Context) error {
	name := strings.ToLower(strings.TrimSpace(c.String("name")))
	if !feedNameRe.MatchString(name) {
		return cmd.Errorf("Feed names can only use lowercase letters, digits, `-` and `_` (max 32).")
	}
	u, err := safehttp.ParseURL(strings.Trim(strings.TrimSpace(c.String("url")), "<>"))
	if err != nil {
		return cmd.Errorf("That URL doesn't work: %s.", err)
	}
	channel := targetChannel(c)
	if err := checkCanPost(c, channel, false); err != nil {
		return err
	}
	st := c.Registry.Store()
	existing, err := st.GuildFeeds(c, *c.GuildID)
	if err != nil {
		return err
	}
	if len(existing) >= maxFeedsPerGuild {
		return cmd.Errorf("This server already has %d feeds, the maximum.", maxFeedsPerGuild)
	}
	for _, f := range existing {
		if f.ChannelID == channel && f.Name == name {
			return cmd.Errorf("<#%s> already has a feed named `%s`.", channel, name)
		}
	}

	if err := c.Defer(); err != nil {
		return err
	}
	res, err := m.fetcher.Fetch(c, u.String(), "", "")
	if err != nil {
		return cmd.Errorf("I couldn't read that feed: %s. Try `/rss find` with the website's address.", err)
	}
	f, err := st.AddFeed(c, store.Feed{
		GuildID:   *c.GuildID,
		ChannelID: channel,
		Name:      name,
		URL:       u.String(),
		Template:  rss.DefaultTemplate,
		// Everything currently in the feed counts as already posted, so
		// subscribing doesn't flood the channel with its backlog.
		Seen:         rss.MarkSeen(res.Feed.Items, nil),
		ETag:         res.ETag,
		LastModified: res.LastModified,
		CreatorID:    c.Author.ID,
		CreatedAt:    time.Now(),
	})
	if errors.Is(err, store.ErrFeedExists) {
		return cmd.Errorf("<#%s> already has a feed named `%s`.", channel, name)
	}
	if err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("✅ <#%s> is now subscribed to **%s** as `%s` (%d items so far). New items will be posted as they appear; use `/rss force %s` to post the latest one now.",
		f.ChannelID, rss.Title(res.Feed, f.URL), f.Name, len(res.Feed.Items), f.Name))
}

func (m *Module) remove(c *cmd.Context) error {
	f, err := resolveFeed(c)
	if err != nil {
		return err
	}
	if err := c.Registry.Store().DeleteFeed(c, f.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return c.Reply(fmt.Sprintf("Removed feed `%s` from <#%s>.", f.Name, f.ChannelID))
}

func (m *Module) list(c *cmd.Context) error {
	feeds, err := c.Registry.Store().GuildFeeds(c, *c.GuildID)
	if err != nil {
		return err
	}
	filter, filtered := c.OptChannel("channel")
	var lines []string
	for _, f := range feeds {
		if filtered && f.ChannelID != filter {
			continue
		}
		mode := "text"
		if f.Embed {
			mode = "embed"
		}
		line := fmt.Sprintf("`%s` → <#%s> · %s · <%s>", f.Name, f.ChannelID, mode, f.URL)
		if !f.LastPost.IsZero() {
			line += fmt.Sprintf(" · last post <t:%d:R>", f.LastPost.Unix())
		}
		if f.LastError != "" {
			line += "\n  ⚠️ " + truncate(f.LastError, 150)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return c.Reply("No feeds yet. Add one with `/rss add`.")
	}
	var pages []cmd.Page
	for i := 0; i < len(lines); i += 10 {
		pages = append(pages, cmd.Page{Embed: &discord.Embed{
			Title:       fmt.Sprintf("RSS feeds (%d)", len(lines)),
			Description: strings.Join(lines[i:min(i+10, len(lines))], "\n"),
			Color:       0xEE802F,
		}})
	}
	return c.Paginate(pages)
}

func (m *Module) template(c *cmd.Context) error {
	f, err := resolveFeed(c)
	if err != nil {
		return err
	}
	tmpl, ok := c.OptString("template")
	if !ok || strings.TrimSpace(tmpl) == "" {
		var vars []string
		for _, v := range rss.Variables {
			vars = append(vars, fmt.Sprintf("`$%s` %s", v.Name, v.Desc))
		}
		mode := ""
		if f.Embed {
			mode = "\n*(This feed posts embeds, so the template is currently unused.)*"
		}
		return c.Reply(fmt.Sprintf("Template for `%s`:\n```\n%s\n```%s\nVariables:\n%s\nUse `\\n` for a new line.",
			f.Name, f.Template, mode, strings.Join(vars, "\n")))
	}
	tmpl = rss.NormalizeTemplate(strings.TrimSpace(tmpl))
	if strings.EqualFold(tmpl, "reset") {
		tmpl = rss.DefaultTemplate
	}
	if err := c.Registry.Store().UpdateFeedSettings(c, f.ID, tmpl, f.Embed); err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("Updated the template for `%s`:\n```\n%s\n```", f.Name, tmpl))
}

func (m *Module) embed(c *cmd.Context) error {
	f, err := resolveFeed(c)
	if err != nil {
		return err
	}
	enabled := c.Bool("enabled")
	if enabled {
		if err := checkCanPost(c, f.ChannelID, true); err != nil {
			return err
		}
	}
	if err := c.Registry.Store().UpdateFeedSettings(c, f.ID, f.Template, enabled); err != nil {
		return err
	}
	state := "text using its template"
	if enabled {
		state = "embeds"
	}
	return c.Reply(fmt.Sprintf("`%s` will now post %s.", f.Name, state))
}

func (m *Module) force(c *cmd.Context) error {
	f, err := resolveFeed(c)
	if err != nil {
		return err
	}
	if err := c.Defer(); err != nil {
		return err
	}
	res, err := m.fetcher.Fetch(c, f.URL, "", "")
	if err != nil {
		return cmd.Errorf("I couldn't read that feed: %s.", err)
	}
	latest := rss.Latest(res.Feed.Items)
	if latest == nil {
		return cmd.Errorf("That feed has no items.")
	}
	if _, err := c.Client.Rest.CreateMessage(f.ChannelID, rss.Message(f.Template, f.Embed, res.Feed, latest, f.Name)); err != nil {
		return cmd.Errorf("I couldn't post in <#%s>: %s", f.ChannelID, err)
	}
	f.Seen = rss.MarkSeen(res.Feed.Items, f.Seen)
	f.LastPost, f.LastError = time.Now(), ""
	if err := c.Registry.Store().UpdateFeedState(c, f); err != nil {
		return err
	}
	if f.ChannelID == c.ChannelID && !c.IsInteraction() {
		return nil // the post itself is the reply
	}
	return c.ReplyEphemeral(fmt.Sprintf("Posted the latest item from `%s` in <#%s>.", f.Name, f.ChannelID))
}

func (m *Module) find(c *cmd.Context) error {
	raw := strings.Trim(strings.TrimSpace(c.String("url")), "<>")
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	if _, err := safehttp.ParseURL(raw); err != nil {
		return cmd.Errorf("That URL doesn't work: %s.", err)
	}
	if err := c.Defer(); err != nil {
		return err
	}
	found, err := m.fetcher.Discover(c, raw)
	if err != nil {
		return cmd.Errorf("I couldn't load that page: %s.", err)
	}
	if len(found) == 0 {
		return cmd.Errorf("I didn't find any feeds advertised on that page.")
	}
	var b strings.Builder
	for _, f := range found[:min(10, len(found))] {
		title := f.Title
		if title == "" {
			title = "Feed"
		}
		fmt.Fprintf(&b, "**%s**\n<%s>\n", truncate(title, 100), f.URL)
	}
	return c.Reply(b.String() + "\nSubscribe with `/rss add name:<name> url:<feed url>`.")
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
