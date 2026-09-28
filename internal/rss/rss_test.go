package rss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/mmcdole/gofeed"

	"mayuri2000/internal/store"
)

func parseFixture(t *testing.T) *gofeed.Feed {
	t.Helper()
	data, err := os.ReadFile("testdata/bonequest.rss")
	if err != nil {
		t.Fatal(err)
	}
	feed, err := gofeed.NewParser().ParseString(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return feed
}

func TestRenderLegacyTemplate(t *testing.T) {
	feed := parseFixture(t)
	latest := Latest(feed.Items)
	if latest == nil || latest.Title != "reward offered" {
		t.Fatalf("unexpected latest item: %+v", latest)
	}
	// The template the old Red bot used for this feed.
	if got := Render(DefaultTemplate, feed, latest, "jerj"); got != "reward offered\nhttps://www.bonequest.com/10055" {
		t.Fatalf("got %q", got)
	}
	got := Render(NormalizeTemplate(`**$title** from $feed ($name)\n$content_image01 $published $unknown`), feed, latest, "jerj")
	for _, want := range []string{"**reward offered** from BoneQuest (jerj)", "https://www.bonequest.com/10055.gif", "<t:", "$unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	e := Embed(feed, latest, "jerj")
	if e.Image == nil || e.Image.URL != "https://www.bonequest.com/10055.gif" || e.URL != latest.Link || e.Timestamp == nil {
		t.Fatalf("bad embed: %+v", e)
	}
	// Feed content must never ping anyone.
	msg := Message(DefaultTemplate, false, feed, latest, "jerj")
	if msg.AllowedMentions == nil || len(msg.AllowedMentions.Parse) != 0 {
		t.Fatalf("mentions not suppressed: %+v", msg.AllowedMentions)
	}
}

func item(id string, published time.Time) *gofeed.Item {
	return &gofeed.Item{GUID: id, Title: id, Link: "https://example.com/" + id, PublishedParsed: &published}
}

func TestNewItems(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Feeds list newest first.
	items := []*gofeed.Item{item("c", base.Add(3*time.Hour)), item("b", base.Add(2*time.Hour)), item("a", base.Add(time.Hour))}
	fresh, seen := NewItems(items, []string{"a"})
	if len(fresh) != 2 || fresh[0].GUID != "b" || fresh[1].GUID != "c" {
		t.Fatalf("expected b, c oldest first; got %v", ids(fresh))
	}
	if strings.Join(seen, ",") != "c,b,a" {
		t.Fatalf("seen = %v", seen)
	}

	var many []*gofeed.Item
	for i := 20; i > 0; i-- {
		many = append(many, item(fmt.Sprint(i), base.Add(time.Duration(i)*time.Hour)))
	}
	fresh, _ = NewItems(many, nil)
	if len(fresh) != MaxPostsPerPoll || fresh[len(fresh)-1].GUID != "20" || fresh[0].GUID != "16" {
		t.Fatalf("flood cap should keep the newest %d: %v", MaxPostsPerPoll, ids(fresh))
	}

	// Undated items fall back to reversed feed order.
	undated := []*gofeed.Item{{GUID: "new"}, {GUID: "old"}}
	fresh, _ = NewItems(undated, nil)
	if ids(fresh) != "old,new" {
		t.Fatalf("undated order: %v", ids(fresh))
	}
}

func ids(items []*gofeed.Item) string {
	var out []string
	for _, it := range items {
		out = append(out, ItemID(it))
	}
	return strings.Join(out, ",")
}

func TestMarkSeenBounded(t *testing.T) {
	var seen []string
	for i := 0; i < 1000; i++ {
		seen = append(seen, fmt.Sprint(i))
	}
	if got := MarkSeen([]*gofeed.Item{{GUID: "x"}}, seen); len(got) != maxSeen || got[0] != "x" {
		t.Fatalf("len %d first %q", len(got), got[0])
	}
}

func TestParseFeedLinks(t *testing.T) {
	base, _ := url.Parse("https://blog.example.com/posts/")
	page := `<html><head>
<link rel="alternate" type="application/rss+xml" title="Posts" href="/feed.xml">
<link rel="alternate" type="application/atom+xml" href="https://blog.example.com/atom.xml">
<link rel="stylesheet" href="/style.css">
<link rel="alternate" type="application/rss+xml" href="/feed.xml">
</head></html>`
	got := ParseFeedLinks([]byte(page), base)
	if len(got) != 2 || got[0].URL != "https://blog.example.com/feed.xml" || got[0].Title != "Posts" || got[1].URL != "https://blog.example.com/atom.xml" {
		t.Fatalf("got %+v", got)
	}
}

// fakePoster records posts and can fail on demand.
type fakePoster struct {
	mu    sync.Mutex
	posts []string
	fail  func(n int) error
}

func (p *fakePoster) CreateMessage(channelID snowflake.ID, msg discord.MessageCreate, _ ...rest.RequestOpt) (*discord.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail != nil {
		if err := p.fail(len(p.posts)); err != nil {
			return nil, err
		}
	}
	p.posts = append(p.posts, msg.Content)
	return &discord.Message{}, nil
}

func rssDoc(items ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title>`)
	for i := len(items) - 1; i >= 0; i-- { // newest first
		fmt.Fprintf(&b, `<item><title>%s</title><link>https://example.com/%s</link><guid>%s</guid><pubDate>%s</pubDate></item>`,
			items[i], items[i], items[i], time.Date(2026, 1, 1, i, 0, 0, 0, time.UTC).Format(time.RFC1123Z))
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func TestPollerEndToEnd(t *testing.T) {
	var mu sync.Mutex
	doc := rssDoc("a", "b")
	requests, notModified := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		etag := fmt.Sprintf(`"%d"`, len(doc))
		if r.Header.Get("If-None-Match") == etag {
			notModified++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = io.WriteString(w, doc)
	}))
	defer srv.Close()
	setDoc := func(s string) { mu.Lock(); doc = s; mu.Unlock() }

	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "rss.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	// Tests talk to a loopback server, which the production client refuses.
	fetcher := &Fetcher{client: srv.Client()}
	res, err := fetcher.Fetch(ctx, srv.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	feed, err := st.AddFeed(ctx, store.Feed{GuildID: 1, ChannelID: 2, Name: "test", URL: srv.URL, Template: DefaultTemplate, Seen: MarkSeen(res.Feed.Items, nil), CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	poster := &fakePoster{}
	p := &Poller{Store: st, Fetcher: fetcher, Poster: poster, Interval: time.Hour, Log: slog.New(slog.DiscardHandler), validators: map[string][2]string{}}

	p.PollOnce(ctx)
	if len(poster.posts) != 0 {
		t.Fatalf("existing items were posted: %v", poster.posts)
	}
	p.PollOnce(ctx)
	if notModified == 0 {
		t.Fatal("expected a conditional GET to return 304")
	}

	setDoc(rssDoc("a", "b", "c", "d"))
	p.PollOnce(ctx)
	if strings.Join(poster.posts, "|") != "c\nhttps://example.com/c|d\nhttps://example.com/d" {
		t.Fatalf("posts = %q", poster.posts)
	}

	// A failed post is retried on the next poll, not lost.
	setDoc(rssDoc("a", "b", "c", "d", "e", "f"))
	poster.fail = func(n int) error {
		if n == 3 {
			return errors.New("discord hiccup")
		}
		return nil
	}
	p.PollOnce(ctx)
	if len(poster.posts) != 3 || !strings.HasPrefix(poster.posts[2], "e") {
		t.Fatalf("after failure posts = %q", poster.posts)
	}
	f, _ := st.Feed(ctx, feed.ID)
	if !strings.Contains(f.LastError, "discord hiccup") {
		t.Fatalf("last error not recorded: %q", f.LastError)
	}
	poster.fail = nil
	p.PollOnce(ctx)
	if len(poster.posts) != 4 || !strings.HasPrefix(poster.posts[3], "f") {
		t.Fatalf("retry posts = %q", poster.posts)
	}
	if f, _ = st.Feed(ctx, feed.ID); f.LastError != "" || f.LastPost.IsZero() {
		t.Fatalf("state after recovery: %+v", f)
	}

	// A deleted channel removes the subscription.
	setDoc(rssDoc("a", "b", "c", "d", "e", "f", "g"))
	poster.fail = func(int) error { return &rest.Error{Code: rest.JSONErrorCodeUnknownChannel} }
	p.PollOnce(ctx)
	if feeds, _ := st.Feeds(ctx); len(feeds) != 0 {
		t.Fatalf("feed for deleted channel was kept: %+v", feeds)
	}
}

func TestFetchRejectsNonFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html><body>not a feed</body></html>")
	}))
	defer srv.Close()
	f := &Fetcher{client: srv.Client()}
	if _, err := f.Fetch(context.Background(), srv.URL, "", ""); err == nil {
		t.Fatal("expected an error for a non-feed page")
	}
	// The production fetcher must refuse loopback addresses outright.
	if _, err := NewFetcher().Fetch(context.Background(), srv.URL, "", ""); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected loopback to be refused, got %v", err)
	}
}
