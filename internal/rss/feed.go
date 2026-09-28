// Package rss fetches RSS/Atom/JSON feeds, tracks which items have been
// posted, and renders new items for Discord.
package rss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"mayuri2000/internal/safehttp"
)

const (
	maxFeedSize = 5 << 20
	// maxSeen bounds the remembered item IDs per feed.
	maxSeen = 500
	// MaxPostsPerPoll caps how many new items one poll posts, so a feed
	// that suddenly republishes everything can't flood a channel.
	MaxPostsPerPoll = 5
	userAgent       = "Mozilla/5.0 (compatible; Mayuri2000 Discord bot; RSS reader)"
)

// ErrNotModified means the server reported the feed unchanged (HTTP 304).
var ErrNotModified = errors.New("feed not modified")

// Fetcher downloads and parses feeds from user-supplied URLs.
type Fetcher struct {
	client *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{client: safehttp.NewClient(30 * time.Second)}
}

// Result is a fetched feed plus the validators for the next conditional GET.
type Result struct {
	Feed         *gofeed.Feed
	ETag         string
	LastModified string
}

// Fetch downloads url. Pass the previous ETag/Last-Modified to allow a 304.
func (f *Fetcher) Fetch(ctx context.Context, url, etag, lastModified string) (*Result, error) {
	req, err := safehttp.NewRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/feed+json, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return nil, ErrNotModified
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxFeedSize {
		return nil, fmt.Errorf("feed is larger than %d MiB", maxFeedSize>>20)
	}
	feed, err := gofeed.NewParser().ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("not a valid RSS, Atom or JSON feed")
	}
	return &Result{Feed: feed, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}, nil
}

// ItemID identifies an item across polls.
func ItemID(it *gofeed.Item) string {
	switch {
	case it.GUID != "":
		return it.GUID
	case it.Link != "":
		return it.Link
	}
	return it.Title + "|" + it.Published
}

func itemTime(it *gofeed.Item) *time.Time {
	if it.PublishedParsed != nil {
		return it.PublishedParsed
	}
	return it.UpdatedParsed
}

// Chronological returns items oldest first. Dated items are sorted by date;
// otherwise feed order (conventionally newest first) is reversed.
func Chronological(items []*gofeed.Item) []*gofeed.Item {
	out := slices.Clone(items)
	slices.Reverse(out)
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := itemTime(out[i]), itemTime(out[j])
		if ti == nil || tj == nil {
			return false
		}
		return ti.Before(*tj)
	})
	return out
}

// Latest returns the newest item, or nil for an empty feed.
func Latest(items []*gofeed.Item) *gofeed.Item {
	c := Chronological(items)
	if len(c) == 0 {
		return nil
	}
	return c[len(c)-1]
}

// NewItems returns the unseen items oldest first, capped at
// MaxPostsPerPoll (keeping the newest), and the updated seen list.
func NewItems(items []*gofeed.Item, seen []string) (fresh []*gofeed.Item, updated []string) {
	known := make(map[string]bool, len(seen))
	for _, id := range seen {
		known[id] = true
	}
	for _, it := range Chronological(items) {
		if !known[ItemID(it)] {
			fresh = append(fresh, it)
		}
	}
	if len(fresh) > MaxPostsPerPoll {
		fresh = fresh[len(fresh)-MaxPostsPerPoll:]
	}
	return fresh, MarkSeen(items, seen)
}

// MarkSeen merges the current items into the seen list, newest first.
func MarkSeen(items []*gofeed.Item, seen []string) []string {
	var updated []string
	have := map[string]bool{}
	chron := Chronological(items)
	for i := len(chron) - 1; i >= 0; i-- {
		id := ItemID(chron[i])
		if !have[id] {
			have[id] = true
			updated = append(updated, id)
		}
	}
	for _, id := range seen {
		if !have[id] {
			have[id] = true
			updated = append(updated, id)
		}
	}
	if len(updated) > maxSeen {
		updated = updated[:maxSeen]
	}
	return updated
}

// Title returns a display title for a feed, falling back to its URL host.
func Title(feed *gofeed.Feed, url string) string {
	if t := strings.TrimSpace(feed.Title); t != "" {
		return t
	}
	return url
}
