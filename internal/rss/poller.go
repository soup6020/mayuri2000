package rss

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/mmcdole/gofeed"

	"mayuri2000/internal/store"
)

// Poster sends a message to a channel (bot.Client.Rest satisfies it).
type Poster interface {
	CreateMessage(channelID snowflake.ID, msg discord.MessageCreate, opts ...rest.RequestOpt) (*discord.Message, error)
}

// Poller periodically checks every stored feed and posts new items.
type Poller struct {
	Store    *store.Store
	Fetcher  *Fetcher
	Poster   Poster
	Interval time.Duration
	Log      *slog.Logger

	// validators caches ETag/Last-Modified per URL, since several channels
	// may subscribe to the same feed.
	mu         sync.Mutex
	validators map[string][2]string
}

// Run polls until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	p.validators = map[string][2]string{}
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		p.PollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PollOnce checks all feeds once, fetching each distinct URL a single time.
func (p *Poller) PollOnce(ctx context.Context) {
	feeds, err := p.Store.Feeds(ctx)
	if err != nil {
		p.Log.Error("rss: loading feeds", "err", err)
		return
	}
	byURL := map[string][]store.Feed{}
	for _, f := range feeds {
		byURL[f.URL] = append(byURL[f.URL], f)
	}

	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for url, subs := range byURL {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			p.pollURL(ctx, url, subs)
		}()
	}
	wg.Wait()
}

func (p *Poller) pollURL(ctx context.Context, url string, subs []store.Feed) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	p.mu.Lock()
	v := p.validators[url]
	p.mu.Unlock()
	res, err := p.Fetcher.Fetch(ctx, url, v[0], v[1])
	if errors.Is(err, ErrNotModified) {
		return
	}
	if err != nil {
		p.Log.Warn("rss: fetch failed", "url", url, "err", err)
		for _, f := range subs {
			if f.LastError != err.Error() {
				f.LastError = err.Error()
				_ = p.Store.UpdateFeedState(ctx, f)
			}
		}
		return
	}
	allDelivered := true
	for _, f := range subs {
		if !p.deliver(ctx, f, res) {
			allDelivered = false
		}
	}
	// Only allow a 304 next time if nothing is waiting to be retried.
	p.mu.Lock()
	if allDelivered {
		p.validators[url] = [2]string{res.ETag, res.LastModified}
	} else {
		delete(p.validators, url)
	}
	p.mu.Unlock()
}

// deliver posts a feed's new items, reporting false if some failed to post.
func (p *Poller) deliver(ctx context.Context, f store.Feed, res *Result) bool {
	fresh, _ := NewItems(res.Feed.Items, f.Seen)
	f.ETag, f.LastModified, f.LastError = res.ETag, res.LastModified, ""
	posted := 0
	for _, it := range fresh {
		_, err := p.Poster.CreateMessage(f.ChannelID, Message(f.Template, f.Embed, res.Feed, it, f.Name))
		if err != nil {
			if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownChannel) {
				p.Log.Info("rss: channel is gone, removing feed", "feed", f.Name, "channel", f.ChannelID)
				_ = p.Store.DeleteFeed(ctx, f.ID)
				return true
			}
			p.Log.Warn("rss: posting failed", "feed", f.Name, "channel", f.ChannelID, "err", err)
			f.LastError = "posting failed: " + err.Error()
			break
		}
		posted++
		f.LastPost = time.Now()
	}
	// Items that failed to post stay unseen so the next poll retries them.
	unposted := map[string]bool{}
	for _, it := range fresh[posted:] {
		unposted[ItemID(it)] = true
	}
	var done []*gofeed.Item
	for _, it := range res.Feed.Items {
		if !unposted[ItemID(it)] {
			done = append(done, it)
		}
	}
	f.Seen = MarkSeen(done, f.Seen)
	if err := p.Store.UpdateFeedState(ctx, f); err != nil {
		p.Log.Error("rss: saving feed state", "feed", f.Name, "err", err)
	}
	return posted == len(fresh)
}
