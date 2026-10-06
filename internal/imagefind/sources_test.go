package imagefind

import (
	"os"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
)

func TestParseTenorMP4(t *testing.T) {
	page, err := os.ReadFile("testdata/tenor_view.html")
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseTenorMP4(string(page)); got != "https://media.tenor.com/WLnIKJZU4XcAAAPo/october-happy-october.mp4" {
		t.Fatalf("got %q", got)
	}
	if got := ParseTenorMP4("<html></html>"); got != "" {
		t.Fatalf("no og:video should give empty, got %q", got)
	}
}

func TestGIFHosts(t *testing.T) {
	for u, want := range map[string]bool{
		"https://media.tenor.com/x/y.mp4":         true,
		"https://media1.tenor.com/m/x/y.gif":      true,
		"https://static2.klipy.com/ii/a/b/c.mp4":  true,
		"https://klipy.com/gifs/x":                true,
		"https://cdn.discordapp.com/a/b/clip.mp4": false,
		"https://nottenor.com/x.mp4":              false,
		"https://tenor.com.evil.example/x.mp4":    false,
	} {
		if got := isGIFHost(u); got != want {
			t.Errorf("isGIFHost(%s) = %v, want %v", u, got, want)
		}
	}
}

func ptr(s string) *string { return &s }

func TestClassifyMessage(t *testing.T) {
	f := New("")
	c, _ := cmd.NewTestContext(nil, nil)
	m := discord.Message{
		Attachments: []discord.Attachment{
			{URL: "https://cdn.discordapp.com/attachments/1/2/cat.png", ContentType: ptr("image/png")},
			{URL: "https://cdn.discordapp.com/attachments/1/2/clip.mp4", ContentType: ptr("video/mp4")},
			{URL: "https://cdn.discordapp.com/attachments/1/2/clip.webm", ContentType: ptr("video/webm")},
			{URL: "https://cdn.discordapp.com/attachments/1/2/notes.txt", ContentType: ptr("text/plain")},
		},
		Embeds: []discord.Embed{
			{Type: discord.EmbedTypeGifV, Video: &discord.EmbedResource{URL: "https://static2.klipy.com/ii/a/b/c.mp4"}},
			{Type: discord.EmbedTypeGifV, Video: &discord.EmbedResource{URL: "https://media.tenor.com/abc/d.mp4"}},
			{Type: discord.EmbedTypeVideo, Video: &discord.EmbedResource{URL: "https://example.com/v/clip.mp4"}},
			{Type: discord.EmbedTypeVideo, Video: &discord.EmbedResource{URL: "https://www.youtube.com/embed/xyz"}},
			{Type: discord.EmbedTypeImage, Thumbnail: &discord.EmbedResource{URL: "https://example.com/pic.jpg"}},
		},
	}
	want := []Source{
		{URL: "https://cdn.discordapp.com/attachments/1/2/cat.png", Kind: Image},
		{URL: "https://cdn.discordapp.com/attachments/1/2/clip.mp4", Kind: Video},
		{URL: "https://static2.klipy.com/ii/a/b/c.mp4", Kind: GIFVideo},
		{URL: "https://media.tenor.com/abc/d.mp4", Kind: GIFVideo},
		{URL: "https://example.com/v/clip.mp4", Kind: Video},
		{URL: "https://example.com/pic.jpg", Kind: Image},
	}
	got := f.fromMessage(c, m)
	if len(got) != len(want) {
		t.Fatalf("got %d sources: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// An MP4 attachment hosted by Tenor/Klipy is still a GIF video.
	if s := attachmentSource(discord.Attachment{URL: "https://media.tenor.com/x.mp4", ContentType: ptr("video/mp4")}); s.Kind != GIFVideo {
		t.Errorf("tenor-hosted attachment kind = %v", s.Kind)
	}
}

func TestTextSources(t *testing.T) {
	f := New("")
	c, _ := cmd.NewTestContext(nil, nil)
	got := f.fromText(c, "look https://example.com/a.png and https://example.com/b.mp4?x=1 and https://media.tenor.com/q/r.mp4", false)
	want := []Source{
		{URL: "https://example.com/a.png", Kind: Image},
		{URL: "https://example.com/b.mp4?x=1", Kind: Video},
		{URL: "https://media.tenor.com/q/r.mp4", Kind: GIFVideo},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Klipy page links are not fetched (they 403); they're resolved through embeds.
	if got := f.fromText(c, "https://klipy.com/gifs/night-night-5", true); len(got) != 0 {
		t.Errorf("klipy page link should not become a source: %+v", got)
	}
}

func TestAwaitEmbeds(t *testing.T) {
	f := New("")
	var slept int
	f.sleep = func(time.Duration) { slept++ }
	fetches := 0
	gifv := discord.Embed{Type: discord.EmbedTypeGifV, Video: &discord.EmbedResource{URL: "https://static2.klipy.com/ii/a/b/c.mp4"}}
	f.getMessage = func(_ *cmd.Context, _, _ snowflake.ID) (*discord.Message, error) {
		fetches++
		m := discord.Message{Content: "%magik https://klipy.com/gifs/c"}
		if fetches >= 2 {
			m.Embeds = []discord.Embed{gifv}
		}
		return &m, nil
	}
	c, _ := cmd.NewTestContext(nil, map[string]any{"url": "https://klipy.com/gifs/c"})
	c.SourceMessage = &discord.Message{Content: "%magik https://klipy.com/gifs/c"}

	srcs, err := f.Find(c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 || srcs[0] != (Source{URL: "https://static2.klipy.com/ii/a/b/c.mp4", Kind: GIFVideo}) {
		t.Fatalf("sources = %+v", srcs)
	}
	if fetches != 2 || slept != 2 {
		t.Fatalf("fetches=%d slept=%d, want the embed on the second re-fetch", fetches, slept)
	}

	// Gives up after the bounded number of re-fetches.
	fetches, slept = 0, 0
	f.getMessage = func(_ *cmd.Context, _, _ snowflake.ID) (*discord.Message, error) {
		fetches++
		return &discord.Message{}, nil
	}
	if m := f.awaitEmbeds(c); len(m.Embeds) != 0 || fetches != embedRefetches {
		t.Fatalf("fetches=%d embeds=%d", fetches, len(m.Embeds))
	}

	// A message that already has its embed isn't re-fetched.
	fetches = 0
	c.SourceMessage = &discord.Message{Embeds: []discord.Embed{gifv}}
	if m := f.awaitEmbeds(c); len(m.Embeds) != 1 || fetches != 0 {
		t.Fatalf("unexpected re-fetch: fetches=%d", fetches)
	}
}
