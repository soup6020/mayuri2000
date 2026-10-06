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
	videoLinkRe = regexp.MustCompile(`(?i)https?://[^"'\s<>]*\.mp4(?:\?[^"'\s<>]*)?`)
	klipyRe     = regexp.MustCompile(`https?://(?:www\.)?klipy\.com/(?:gifs|clips|stickers)/[^\s<>]+`)
	// Tenor's tags carry other attributes (e.g. class) before property.
	tenorMP4Re = regexp.MustCompile(`<meta[^>]*\sproperty="og:video"[^>]*\scontent="(https://media\d*\.tenor\.com/[^"]+\.mp4)"`)
)

// Kind says what a source is and how its output should look.
type Kind int

const (
	// Image is a still or animated image (or a link of unknown type).
	Image Kind = iota
	// Video is an MP4 video; output is MP4 with its audio.
	Video
	// GIFVideo is a Tenor or Klipy GIF served as MP4; output is a GIF.
	GIFVideo
)

// Source is an input found for a command.
type Source struct {
	URL  string
	Kind Kind
}

// isGIFHost reports whether a URL is served by Tenor or Klipy, whose MP4s
// are GIFs posted from Discord's picker.
func isGIFHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, d := range []string{"tenor.com", "klipy.com"} {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// videoSource classifies an MP4 URL by its host.
func videoSource(u string) Source {
	if isGIFHost(u) {
		return Source{URL: u, Kind: GIFVideo}
	}
	return Source{URL: u, Kind: Video}
}

// linkSource classifies an arbitrary link; its content decides later
// whether it is an image or a video.
func linkSource(u string) Source {
	if isGIFHost(u) {
		return Source{URL: u, Kind: GIFVideo}
	}
	return Source{URL: u, Kind: Image}
}

func images(urls ...string) []Source {
	out := make([]Source, 0, len(urls))
	for _, u := range urls {
		out = append(out, Source{URL: u, Kind: Image})
	}
	return out
}

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

	// getMessage and sleep are overridable for tests; by default they use
	// the command's REST client and time.Sleep.
	getMessage func(c *cmd.Context, channelID, messageID snowflake.ID) (*discord.Message, error)
	sleep      func(time.Duration)
}

func New(tenorKey string) *Finder {
	return &Finder{
		TenorKey: tenorKey,
		http:     &http.Client{Timeout: 15 * time.Second},
		getMessage: func(c *cmd.Context, channelID, messageID snowflake.ID) (*discord.Message, error) {
			return c.Client.Rest.GetMessage(channelID, messageID)
		},
		sleep: time.Sleep,
	}
}

var ErrNotFound = cmd.Errorf("I couldn't find an image or MP4 video. Attach one, pass a URL, emoji or user, or reply to a message with one.")

// Embed wait for links in the command's own message: Discord usually adds
// the GIF embed a moment after the message is created.
const (
	embedRefetches = 3
	embedWait      = time.Second
)

// Find returns up to limit sources for the command, searching explicit
// arguments first, then replies/targets and finally recent channel history.
func (f *Finder) Find(c *cmd.Context, limit int) ([]Source, error) {
	var srcs []Source
	add := func(ss ...Source) {
		for _, s := range ss {
			if s.URL != "" && len(srcs) < limit {
				srcs = append(srcs, s)
			}
		}
	}

	if a, ok := c.OptAttachment("image"); ok {
		add(attachmentSource(a))
	}
	text := c.String("url")
	if len(c.Extras) > 0 {
		text = strings.TrimSpace(text + " " + strings.Join(c.Extras, " "))
	}
	if text != "" {
		// A Tenor/Klipy link in the command's own message: prefer the GIF
		// embed Discord attaches to it (Klipy pages can't be fetched).
		if c.SourceMessage != nil && (tenorRe.MatchString(text) || klipyRe.MatchString(text)) {
			add(f.embedsOf(f.awaitEmbeds(c))...)
		}
		add(f.fromText(c, text, true)...)
	}
	if u, ok := c.OptUser("user"); ok {
		add(Source{URL: Avatar(u)})
	}
	if c.TargetUser != nil {
		add(Source{URL: Avatar(*c.TargetUser)})
	}
	if m := c.SourceMessage; m != nil {
		// The first attachment was already taken as the "image" option.
		if len(m.Attachments) > 1 {
			add(f.fromMessage(c, discord.Message{Attachments: m.Attachments[1:]})...)
		}
	}
	if len(srcs) > 0 {
		return srcs, nil
	}

	if c.TargetMessage != nil {
		add(f.fromMessage(c, *c.TargetMessage)...)
	}
	if m := c.SourceMessage; m != nil && m.ReferencedMessage != nil {
		add(f.fromMessage(c, *m.ReferencedMessage)...)
	}
	if len(srcs) > 0 {
		return srcs, nil
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
	if len(srcs) == 0 {
		return nil, ErrNotFound
	}
	return srcs, nil
}

// awaitEmbeds re-fetches the command's message until Discord has attached
// embeds to it (or gives up), returning the latest copy.
func (f *Finder) awaitEmbeds(c *cmd.Context) discord.Message {
	m := *c.SourceMessage
	for i := 0; i < embedRefetches && len(m.Embeds) == 0; i++ {
		f.sleep(embedWait)
		if c.Err() != nil {
			break
		}
		if fresh, err := f.getMessage(c, m.ChannelID, m.ID); err == nil && fresh != nil {
			m = *fresh
		}
	}
	return m
}

func attachmentSource(a discord.Attachment) Source {
	if a.ContentType != nil && *a.ContentType == "video/mp4" {
		return videoSource(a.URL)
	}
	return Source{URL: a.URL, Kind: Image}
}

// embedsOf returns the media of a message's embeds: GIF embeds (Tenor and
// Klipy posts) as GIF videos, video embeds as videos, and images.
func (f *Finder) embedsOf(m discord.Message) []Source {
	var srcs []Source
	for _, e := range m.Embeds {
		switch {
		case e.Type == discord.EmbedTypeGifV && e.Video != nil && e.Video.URL != "":
			srcs = append(srcs, Source{URL: e.Video.URL, Kind: GIFVideo})
		case e.Type == discord.EmbedTypeVideo && e.Video != nil && strings.Contains(strings.ToLower(e.Video.URL), ".mp4"):
			srcs = append(srcs, videoSource(e.Video.URL))
		case e.Image != nil && e.Image.URL != "":
			srcs = append(srcs, Source{URL: e.Image.URL, Kind: Image})
		case e.Thumbnail != nil && e.Thumbnail.URL != "" && e.Type == discord.EmbedTypeImage:
			srcs = append(srcs, Source{URL: e.Thumbnail.URL, Kind: Image})
		}
	}
	return srcs
}

func (f *Finder) fromMessage(c *cmd.Context, m discord.Message) []Source {
	var srcs []Source
	for _, a := range m.Attachments {
		if a.ContentType == nil {
			continue
		}
		if ct := *a.ContentType; strings.HasPrefix(ct, "image/") || ct == "video/mp4" {
			srcs = append(srcs, attachmentSource(a))
		}
	}
	embeds := f.embedsOf(m)
	srcs = append(srcs, embeds...)
	// Without an embed, fall back to the message text (e.g. Tenor links
	// whose embed hasn't loaded).
	if len(embeds) == 0 {
		srcs = append(srcs, f.fromText(c, m.Content, false)...)
	}
	return srcs
}

// fromText extracts image references from free text. When explicit is set
// (the user typed it as an argument) mentions, raw IDs and arbitrary links
// are honoured too.
func (f *Finder) fromText(c *cmd.Context, text string, explicit bool) []Source {
	var srcs []Source
	for _, m := range tenorRe.FindAllStringSubmatch(text, -1) {
		if u := f.tenorMP4(c, m[0], m[1]); u != "" {
			srcs = append(srcs, Source{URL: u, Kind: GIFVideo})
		}
	}
	srcs = append(srcs, images(imageLinkRe.FindAllString(text, -1)...)...)
	for _, u := range videoLinkRe.FindAllString(text, -1) {
		srcs = append(srcs, videoSource(u))
	}
	for _, m := range emojiRe.FindAllStringSubmatch(text, -1) {
		ext := "png"
		if m[1] == "a" {
			ext = "gif"
		}
		srcs = append(srcs, Source{URL: fmt.Sprintf("https://cdn.discordapp.com/emojis/%s.%s?size=512", m[2], ext)})
	}
	if !explicit {
		return srcs
	}
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		if u, ok := user(c, m[1]); ok {
			srcs = append(srcs, Source{URL: Avatar(u)})
		}
	}
	if len(srcs) == 0 {
		for _, tok := range strings.Fields(text) {
			// Klipy pages block bots; their GIFs come through embeds.
			if anyLinkRe.MatchString(tok) && !tenorRe.MatchString(tok) && !klipyRe.MatchString(tok) {
				srcs = append(srcs, linkSource(tok))
			}
		}
	}
	if len(srcs) == 0 {
		for _, id := range idRe.FindAllString(text, -1) {
			if u, ok := user(c, id); ok {
				srcs = append(srcs, Source{URL: Avatar(u)})
			}
		}
	}
	return srcs
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

// tenorMP4 resolves a Tenor view link to the GIF's MP4 rendition.
func (f *Finder) tenorMP4(ctx context.Context, link, id string) string {
	if f.TenorKey != "" {
		q := url.Values{"ids": {id}, "key": {f.TenorKey}, "client_key": {"mayuri2000"}, "media_filter": {"mp4"}}
		var out struct {
			Results []struct {
				MediaFormats map[string]struct {
					URL string `json:"url"`
				} `json:"media_formats"`
			} `json:"results"`
		}
		if err := f.getJSON(ctx, "https://tenor.googleapis.com/v2/posts?"+q.Encode(), &out); err == nil {
			for _, r := range out.Results {
				if g, ok := r.MediaFormats["mp4"]; ok && g.URL != "" {
					return g.URL
				}
			}
		}
	}
	// Without an API key, read the MP4 from the view page's og:video.
	body, err := f.get(ctx, link)
	if err != nil {
		return ""
	}
	return ParseTenorMP4(body)
}

// ParseTenorMP4 extracts the MP4 URL from a Tenor view page.
func ParseTenorMP4(page string) string {
	if m := tenorMP4Re.FindStringSubmatch(page); m != nil {
		return m[1]
	}
	return ""
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
