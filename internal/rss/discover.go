package rss

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"

	"mayuri2000/internal/safehttp"
)

var feedTypes = []string{"application/rss+xml", "application/atom+xml", "application/feed+json", "application/json"}

// DiscoveredFeed is a feed advertised by a web page.
type DiscoveredFeed struct {
	Title string
	URL   string
}

// Discover finds the feeds a page links to with <link rel="alternate">. If
// the URL is itself a feed, it is returned directly.
func (f *Fetcher) Discover(ctx context.Context, pageURL string) ([]DiscoveredFeed, error) {
	req, err := safehttp.NewRequest(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedSize))
	if err != nil {
		return nil, err
	}
	if feed, err := gofeed.NewParser().ParseString(string(body)); err == nil {
		return []DiscoveredFeed{{Title: Title(feed, pageURL), URL: resp.Request.URL.String()}}, nil
	}
	return ParseFeedLinks(body, resp.Request.URL), nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "HTTP " + http.StatusText(e.code) }

// ParseFeedLinks extracts <link rel="alternate"> feed links from HTML,
// resolved against base.
func ParseFeedLinks(page []byte, base *url.URL) []DiscoveredFeed {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil
	}
	var out []DiscoveredFeed
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, typ, href, title string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "rel":
					rel = strings.ToLower(a.Val)
				case "type":
					typ = strings.ToLower(strings.TrimSpace(a.Val))
				case "href":
					href = a.Val
				case "title":
					title = a.Val
				}
			}
			isFeed := false
			for _, t := range feedTypes {
				if typ == t {
					isFeed = true
				}
			}
			if strings.Contains(rel, "alternate") && isFeed && href != "" {
				if u, err := base.Parse(href); err == nil && !seen[u.String()] {
					seen[u.String()] = true
					out = append(out, DiscoveredFeed{Title: strings.TrimSpace(title), URL: u.String()})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}
