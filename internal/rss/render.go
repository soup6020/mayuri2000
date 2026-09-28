package rss

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/mmcdole/gofeed"

	"mayuri2000/internal/htmlmd"
)

// DefaultTemplate matches the old Red RSS cog's default.
const DefaultTemplate = "$title\n$link"

// Variables documents the template variables.
var Variables = []struct{ Name, Desc string }{
	{"title", "item title"},
	{"link", "item link"},
	{"summary", "item summary as markdown (truncated)"},
	{"author", "item author"},
	{"published", "publish time as a Discord timestamp"},
	{"categories", "item categories/tags"},
	{"image", "item image URL, if any"},
	{"feed", "feed title"},
	{"name", "subscription name"},
}

// legacy maps variables from Red's RSS cog to ours.
var legacy = map[string]string{
	"summary_plaintext":        "summary",
	"summary_detail_plaintext": "summary",
	"description_plaintext":    "summary",
	"content_plaintext":        "summary",
	"published_parsed":         "published",
	"content_image01":          "image",
	"media_content_plaintext":  "image",
	"tags":                     "categories",
}

var varRe = regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)

var imgSrcRe = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

// Image finds an image for the item: its media image, an image enclosure,
// or the first <img> in its content.
func Image(it *gofeed.Item) string {
	if it.Image != nil && it.Image.URL != "" {
		return it.Image.URL
	}
	for _, e := range it.Enclosures {
		if strings.HasPrefix(e.Type, "image/") && e.URL != "" {
			return e.URL
		}
	}
	if media, ok := it.Extensions["media"]; ok {
		for _, key := range []string{"thumbnail", "content"} {
			for _, ext := range media[key] {
				if u := ext.Attrs["url"]; u != "" && (key == "thumbnail" || strings.HasPrefix(ext.Attrs["medium"], "image") || strings.HasPrefix(ext.Attrs["type"], "image/")) {
					return u
				}
			}
		}
	}
	for _, html := range []string{it.Content, it.Description} {
		if m := imgSrcRe.FindStringSubmatch(html); m != nil {
			return m[1]
		}
	}
	return ""
}

func summary(it *gofeed.Item, limit int) string {
	src := it.Description
	if strings.TrimSpace(src) == "" {
		src = it.Content
	}
	md := htmlmd.Convert(src)
	if r := []rune(md); len(r) > limit {
		md = strings.TrimSpace(string(r[:limit-1])) + "…"
	}
	return md
}

func author(it *gofeed.Item) string {
	var names []string
	for _, a := range it.Authors {
		if a != nil && a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

func values(feed *gofeed.Feed, it *gofeed.Item, name string) map[string]string {
	published := ""
	if t := itemTime(it); t != nil {
		published = fmt.Sprintf("<t:%d:f>", t.Unix())
	}
	return map[string]string{
		"title":      strings.TrimSpace(it.Title),
		"link":       it.Link,
		"summary":    summary(it, 1000),
		"author":     author(it),
		"published":  published,
		"categories": strings.Join(it.Categories, ", "),
		"image":      Image(it),
		"feed":       strings.TrimSpace(feed.Title),
		"name":       name,
	}
}

// NormalizeTemplate turns typed "\n" sequences into newlines, since slash
// command options can't contain real line breaks.
func NormalizeTemplate(t string) string {
	return strings.ReplaceAll(t, `\n`, "\n")
}

// Render fills a text template for an item. Unknown variables are left as-is.
func Render(template string, feed *gofeed.Feed, it *gofeed.Item, name string) string {
	vals := values(feed, it, name)
	out := varRe.ReplaceAllStringFunc(template, func(m string) string {
		key := strings.ToLower(m[1:])
		if alias, ok := legacy[key]; ok {
			key = alias
		}
		if v, ok := vals[key]; ok {
			return v
		}
		return m
	})
	out = strings.TrimSpace(out)
	if r := []rune(out); len(r) > 2000 {
		out = string(r[:1999]) + "…"
	}
	return out
}

// Embed renders an item as a rich embed.
func Embed(feed *gofeed.Feed, it *gofeed.Item, name string) discord.Embed {
	title := strings.TrimSpace(it.Title)
	if title == "" {
		title = "(untitled)"
	}
	e := discord.Embed{
		Title:       truncate(title, 256),
		URL:         it.Link,
		Description: summary(it, 2000),
		Color:       0xEE802F,
		Footer:      &discord.EmbedFooter{Text: truncate(Title(feed, name)+" • "+name, 2048)},
		Timestamp:   itemTime(it),
	}
	if a := author(it); a != "" {
		e.Author = &discord.EmbedAuthor{Name: truncate(a, 256)}
	}
	if img := Image(it); strings.HasPrefix(img, "http") {
		e.Image = &discord.EmbedResource{URL: img}
	}
	return e
}

// Message builds the Discord message for an item.
func Message(template string, embed bool, feed *gofeed.Feed, it *gofeed.Item, name string) discord.MessageCreate {
	msg := discord.MessageCreate{AllowedMentions: &discord.AllowedMentions{}}
	if embed {
		msg.Embeds = []discord.Embed{Embed(feed, it, name)}
	} else {
		msg.Content = Render(template, feed, it, name)
		if msg.Content == "" {
			msg.Content = it.Link
		}
	}
	return msg
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
