// Package imagefind locates the image a command should operate on, a port of
// NotSoBot's ImageFinder converter.
package imagefind

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
)

var (
	imageLinkRe = regexp.MustCompile(`(?i)https?://[^"'\s<>]*\.(?:png|jpe?g|gif|webp)(?:\?[^"'\s<>]*)?`)
	anyLinkRe   = regexp.MustCompile(`^https?://\S+$`)
	tenorRe     = regexp.MustCompile(`https?://tenor\.com/view/[a-zA-Z0-9-]+-(\d+)`)
	emojiRe     = regexp.MustCompile(`<(a?):[a-zA-Z0-9_]+:(\d+)>`)
	mentionRe   = regexp.MustCompile(`<@!?(\d{15,21})>`)
	idRe        = regexp.MustCompile(`\b\d{17,21}\b`)
	tenorGifRe  = regexp.MustCompile(`https://media\d*\.tenor\.com/[A-Za-z0-9_/-]+\.gif`)
)

// LooksLikeImageRef reports whether a prefix token is plausibly an image
// reference (URL, emoji, mention or ID) rather than another argument.
func LooksLikeImageRef(tok string) bool {
	return anyLinkRe.MatchString(tok) || emojiRe.MatchString(tok) || mentionRe.MatchString(tok) || idRe.MatchString(tok)
}

// Standard options shared by image commands.
func Options(extra ...cmd.Option) []cmd.Option {
	return append([]cmd.Option{
		{Name: "image", Description: "Image to use", Type: cmd.Attachment},
		{Name: "url", Description: "Image URL, custom emoji, or user mention/ID", Type: cmd.String, Accept: LooksLikeImageRef},
		{Name: "user", Description: "Use this user's avatar", Type: cmd.User, SlashOnly: true},
	}, extra...)
}

type Finder struct {
	TenorKey string
	http     *http.Client
}

func New(tenorKey string) *Finder {
	return &Finder{TenorKey: tenorKey, http: &http.Client{Timeout: 15 * time.Second}}
}

var ErrNotFound = cmd.Errorf("I couldn't find an image. Attach one, pass a URL, emoji or user, or reply to a message with an image.")

// Find returns up to limit image URLs for the command, searching explicit
// arguments first, then replies/targets and finally recent channel history.
func (f *Finder) Find(c *cmd.Context, limit int) ([]string, error) {
	var urls []string
	add := func(u ...string) {
		for _, x := range u {
			if x != "" && len(urls) < limit {
				urls = append(urls, x)
			}
		}
	}

	if a, ok := c.OptAttachment("image"); ok {
		add(a.URL)
	}
	text := c.String("url")
	if len(c.Extras) > 0 {
		text = strings.TrimSpace(text + " " + strings.Join(c.Extras, " "))
	}
	if text != "" {
		add(f.fromText(c, text, true)...)
	}
	if u, ok := c.OptUser("user"); ok {
		add(Avatar(u))
	}
	if c.TargetUser != nil {
		add(Avatar(*c.TargetUser))
	}
	if m := c.SourceMessage; m != nil {
		// The first attachment was already taken as the "image" option.
		if len(m.Attachments) > 1 {
			add(f.fromMessage(c, discord.Message{Attachments: m.Attachments[1:]})...)
		}
	}
	if len(urls) > 0 {
		return urls, nil
	}

	if c.TargetMessage != nil {
		add(f.fromMessage(c, *c.TargetMessage)...)
	}
	if m := c.SourceMessage; m != nil && m.ReferencedMessage != nil {
		add(f.fromMessage(c, *m.ReferencedMessage)...)
	}
	if len(urls) > 0 {
		return urls, nil
	}

	var before snowflake.ID
	if c.SourceMessage != nil {
		before = c.SourceMessage.ID
	}
	msgs, err := c.Client.Rest.GetMessages(c.ChannelID, 0, before, 0, 10)
	if err == nil {
		for _, m := range msgs {
			add(f.fromMessage(c, m)...)
		}
	}
	if len(urls) == 0 {
		return nil, ErrNotFound
	}
	return urls, nil
}

func (f *Finder) fromMessage(c *cmd.Context, m discord.Message) []string {
	var urls []string
	for _, a := range m.Attachments {
		if a.ContentType != nil && strings.HasPrefix(*a.ContentType, "image/") {
			urls = append(urls, a.URL)
		}
	}
	for _, e := range m.Embeds {
		switch {
		case e.Image != nil && e.Image.URL != "":
			urls = append(urls, e.Image.URL)
		case e.Provider != nil && strings.EqualFold(e.Provider.Name, "tenor"):
			// handled via the content link below
		case e.Thumbnail != nil && e.Thumbnail.URL != "" && e.Type == discord.EmbedTypeImage:
			urls = append(urls, e.Thumbnail.URL)
		}
	}
	urls = append(urls, f.fromText(c, m.Content, false)...)
	return urls
}

// fromText extracts image references from free text. When explicit is set
// (the user typed it as an argument) mentions, raw IDs and arbitrary links
// are honoured too.
func (f *Finder) fromText(c *cmd.Context, text string, explicit bool) []string {
	var urls []string
	for _, m := range tenorRe.FindAllStringSubmatch(text, -1) {
		if u := f.tenorGIF(c, m[0], m[1]); u != "" {
			urls = append(urls, u)
		}
	}
	urls = append(urls, imageLinkRe.FindAllString(text, -1)...)
	for _, m := range emojiRe.FindAllStringSubmatch(text, -1) {
		ext := "png"
		if m[1] == "a" {
			ext = "gif"
		}
		urls = append(urls, fmt.Sprintf("https://cdn.discordapp.com/emojis/%s.%s?size=512", m[2], ext))
	}
	if !explicit {
		return urls
	}
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		if u, ok := user(c, m[1]); ok {
			urls = append(urls, Avatar(u))
		}
	}
	if len(urls) == 0 {
		for _, tok := range strings.Fields(text) {
			if anyLinkRe.MatchString(tok) && !tenorRe.MatchString(tok) {
				urls = append(urls, tok)
			}
		}
	}
	if len(urls) == 0 {
		for _, id := range idRe.FindAllString(text, -1) {
			if u, ok := user(c, id); ok {
				urls = append(urls, Avatar(u))
			}
		}
	}
	return urls
}

func user(c *cmd.Context, raw string) (discord.User, bool) {
	id, err := snowflake.Parse(raw)
	if err != nil {
		return discord.User{}, false
	}
	if c.GuildID != nil {
		if m, ok := c.Client.Caches.Member(*c.GuildID, id); ok {
			return m.User, true
		}
	}
	u, err := c.Client.Rest.GetUser(id)
	if err != nil {
		return discord.User{}, false
	}
	return *u, true
}

// Avatar returns a user's avatar URL, as a GIF when animated.
func Avatar(u discord.User) string {
	return u.EffectiveAvatarURL(discord.WithSize(1024))
}

func (f *Finder) tenorGIF(ctx context.Context, link, id string) string {
	if f.TenorKey != "" {
		q := url.Values{"ids": {id}, "key": {f.TenorKey}, "client_key": {"mayuri2000"}, "media_filter": {"gif"}}
		var out struct {
			Results []struct {
				MediaFormats map[string]struct {
					URL string `json:"url"`
				} `json:"media_formats"`
			} `json:"results"`
		}
		if err := f.getJSON(ctx, "https://tenor.googleapis.com/v2/posts?"+q.Encode(), &out); err == nil {
			for _, r := range out.Results {
				if g, ok := r.MediaFormats["gif"]; ok && g.URL != "" {
					return g.URL
				}
			}
		}
	}
	// Without an API key, scrape the GIF URL from the view page.
	body, err := f.get(ctx, link)
	if err != nil {
		return ""
	}
	return tenorGifRe.FindString(body)
}

func (f *Finder) get(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Mayuri2000 Discord bot)")
	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

func (f *Finder) getJSON(ctx context.Context, u string, v any) error {
	body, err := f.get(ctx, u)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}
