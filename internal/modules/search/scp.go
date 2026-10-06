package search

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"golang.org/x/net/html"

	"mayuri2000/internal/cmd"
)

const (
	scpWiki          = "https://scp-wiki.wikidot.com/"
	scpMaxNumber     = 9999
	scpIndexCacheTTL = 6 * time.Hour
)

// SCPPage returns the wiki page name for an SCP number, zero-padded to at
// least three digits (49 → "scp-049").
func SCPPage(n int) string { return fmt.Sprintf("scp-%03d", n) }

// SCPSeriesPage returns the series index page listing an SCP number:
// "scp-series" for 1-999, then "scp-series-2" for 1000-1999 and so on.
func SCPSeriesPage(n int) string {
	if n < 1000 {
		return "scp-series"
	}
	return fmt.Sprintf("scp-series-%d", n/1000+1)
}

var scpRatingRe = regexp.MustCompile(`<span class="rate-points">[^<]*<span class="number[^"]*">([+-]?\d+)</span>`)

// ParseSCPRating extracts the article's rating (e.g. "+3029"), or "" if the
// page has no rating widget.
func ParseSCPRating(page []byte) string {
	if m := scpRatingRe.FindSubmatch(page); m != nil {
		return string(m[1])
	}
	return ""
}

var scpLinkRe = regexp.MustCompile(`^/scp-(\d+)$`)

// ParseSCPNames reads a series index page into number → article name.
// Entries look like `<li><a href="/scp-3000">SCP-3000</a> - Anantashesha</li>`;
// formatting inside the name is stripped.
func ParseSCPNames(index []byte) map[int]string {
	names := map[int]string{}
	doc, err := html.Parse(bytes.NewReader(index))
	if err != nil {
		return names
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" {
			if a := n.FirstChild; a != nil && a.Type == html.ElementNode && a.Data == "a" {
				if m := scpLinkRe.FindStringSubmatch(attr(a, "href")); m != nil {
					num, _ := strconv.Atoi(m[1])
					var b strings.Builder
					for c := a.NextSibling; c != nil; c = c.NextSibling {
						b.WriteString(nodeText(c))
					}
					name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(b.String()), "-"))
					if name != "" {
						names[num] = strings.Join(strings.Fields(name), " ")
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return names
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}

// SCPArticle is the result of a lookup.
type SCPArticle struct {
	Number int
	Name   string // "" when the series index has no name for it
	Rating string // "" when the page has no rating widget
	URL    string
}

// Title renders "SCP-3000 — Anantashesha", or "SCP-3000" without a name.
func (a SCPArticle) Title() string {
	t := fmt.Sprintf("SCP-%03d", a.Number)
	if a.Name != "" {
		t += " — " + a.Name
	}
	return t
}

type scpIndex struct {
	names   map[int]string
	fetched time.Time
}

// scpClient looks up SCP articles, caching series index pages.
type scpClient struct {
	fetch func(ctx context.Context, url string) ([]byte, error)
	now   func() time.Time

	mu      sync.Mutex
	indexes map[string]scpIndex
}

func newSCPClient() *scpClient {
	return &scpClient{fetch: getBrowser, now: time.Now, indexes: map[string]scpIndex{}}
}

var errSCPUnavailable = cmd.Errorf("The SCP wiki isn't responding right now.")

// Lookup fetches an article's rating and its name from the (cached) series
// index. Missing articles and outages become user-facing errors; a failed
// index fetch only drops the name.
func (s *scpClient) Lookup(ctx context.Context, n int) (SCPArticle, error) {
	if n < 1 || n > scpMaxNumber {
		return SCPArticle{}, cmd.Errorf("SCP numbers must be between 1 and %d.", scpMaxNumber)
	}
	a := SCPArticle{Number: n, URL: scpWiki + SCPPage(n)}
	page, err := s.fetch(ctx, a.URL)
	var status *HTTPStatusError
	switch {
	case errors.As(err, &status) && status.Code == http.StatusNotFound:
		return SCPArticle{}, cmd.Errorf("SCP-%03d doesn't exist yet.", n)
	case err != nil:
		return SCPArticle{}, errSCPUnavailable
	}
	a.Rating = ParseSCPRating(page)
	a.Name = s.names(ctx, SCPSeriesPage(n))[n]
	return a, nil
}

func (s *scpClient) names(ctx context.Context, series string) map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx, ok := s.indexes[series]; ok && s.now().Sub(idx.fetched) < scpIndexCacheTTL {
		return idx.names
	}
	page, err := s.fetch(ctx, scpWiki+series)
	if err != nil {
		return nil // fall back to the bare number
	}
	idx := scpIndex{names: ParseSCPNames(page), fetched: s.now()}
	s.indexes[series] = idx
	return idx.names
}

// SCPEmbed renders a lookup result.
func SCPEmbed(a SCPArticle) discord.Embed {
	e := discord.Embed{
		Title:  truncate(a.Title(), 256),
		URL:    a.URL,
		Color:  0x8B0000,
		Footer: &discord.EmbedFooter{Text: "SCP Foundation · CC BY-SA 3.0"},
	}
	if a.Rating != "" {
		e.Description = "Rating: **" + a.Rating + "**"
	}
	return e
}

func (s *Searcher) scpArticle(c *cmd.Context) error {
	a, err := s.scp.Lookup(c, c.Int("number"))
	if err != nil {
		return err
	}
	return c.ReplyEmbeds(SCPEmbed(a))
}
