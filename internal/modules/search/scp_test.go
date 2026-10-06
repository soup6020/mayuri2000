package search

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"mayuri2000/internal/cmd"
)

func TestSCPPages(t *testing.T) {
	for n, want := range map[int][2]string{
		1:    {"scp-001", "scp-series"},
		49:   {"scp-049", "scp-series"},
		999:  {"scp-999", "scp-series"},
		1000: {"scp-1000", "scp-series-2"},
		3000: {"scp-3000", "scp-series-4"},
		9999: {"scp-9999", "scp-series-10"},
	} {
		if got := [2]string{SCPPage(n), SCPSeriesPage(n)}; got != want {
			t.Errorf("%d → %v, want %v", n, got, want)
		}
	}
}

func TestParseSCPRating(t *testing.T) {
	page, err := os.ReadFile("testdata/scp_article.html")
	if err != nil {
		t.Fatal(err)
	}
	if got := ParseSCPRating(page); got != "+3029" {
		t.Fatalf("rating = %q, want +3029", got)
	}
	if got := ParseSCPRating([]byte(`<span class="rate-points">rating:&nbsp;<span class="number prw1">-12</span></span>`)); got != "-12" {
		t.Fatalf("negative rating = %q", got)
	}
	if got := ParseSCPRating([]byte("<html>no widget</html>")); got != "" {
		t.Fatalf("missing rating = %q", got)
	}
}

func TestParseSCPNames(t *testing.T) {
	index, err := os.ReadFile("testdata/scp_series.html")
	if err != nil {
		t.Fatal(err)
	}
	names := ParseSCPNames(index)
	for n, want := range map[int]string{
		3000: "Anantashesha",
		3001: "Red Reality",
		3155: "We Never Sleep",   // name wrapped in <em>
		3084: "[DATA EXORCISED]", // nested span/strong/tt formatting
	} {
		if got := names[n]; got != want {
			t.Errorf("names[%d] = %q, want %q", n, got, want)
		}
	}
	if len(names) < 150 {
		t.Errorf("only %d names parsed from the fixture", len(names))
	}
	if got := ParseSCPNames([]byte("<ul><li><a href=\"/scp-5\">SCP-005</a></li><li>stray</li></ul>")); len(got) != 0 {
		t.Errorf("entries without names should be skipped, got %v", got)
	}
}

// fakeWiki serves fixture pages and counts requests per URL. Missing pages
// answer 404, like the real wiki (e.g. /scp-12345).
type fakeWiki struct {
	pages map[string][]byte
	down  bool
	hits  map[string]int
}

func (f *fakeWiki) fetch(_ context.Context, url string) ([]byte, error) {
	f.hits[url]++
	if f.down {
		return nil, errors.New("connection refused")
	}
	if p, ok := f.pages[url]; ok {
		return p, nil
	}
	return nil, &HTTPStatusError{Host: "scp-wiki.wikidot.com", Code: http.StatusNotFound}
}

func newFakeSCP(t *testing.T) (*scpClient, *fakeWiki) {
	t.Helper()
	article, _ := os.ReadFile("testdata/scp_article.html")
	series, _ := os.ReadFile("testdata/scp_series.html")
	w := &fakeWiki{hits: map[string]int{}, pages: map[string][]byte{
		scpWiki + "scp-3000":     article,
		scpWiki + "scp-3001":     article,
		scpWiki + "scp-049":      []byte("<html>no rating here</html>"),
		scpWiki + "scp-series-4": series,
	}}
	c := newSCPClient()
	c.fetch = w.fetch
	return c, w
}

func TestSCPLookup(t *testing.T) {
	c, w := newFakeSCP(t)
	a, err := c.Lookup(context.Background(), 3000)
	if err != nil || a.Title() != "SCP-3000 — Anantashesha" || a.Rating != "+3029" || a.URL != "https://scp-wiki.wikidot.com/scp-3000" {
		t.Fatalf("lookup 3000: %+v, %v", a, err)
	}

	// The series index is cached: a second article in the same series
	// fetches only the article page.
	if _, err := c.Lookup(context.Background(), 3001); err != nil {
		t.Fatal(err)
	}
	if w.hits[scpWiki+"scp-series-4"] != 1 {
		t.Fatalf("series index fetched %d times, want 1", w.hits[scpWiki+"scp-series-4"])
	}
	// ...until the cache expires.
	c.now = func() time.Time { return time.Now().Add(scpIndexCacheTTL + time.Minute) }
	_, _ = c.Lookup(context.Background(), 3000)
	if w.hits[scpWiki+"scp-series-4"] != 2 {
		t.Fatal("expired index was not refetched")
	}
}

func TestSCPLookupFallbacksAndErrors(t *testing.T) {
	c, w := newFakeSCP(t)

	// Index page missing (scp-series not in the fake): bare title, no rating.
	a, err := c.Lookup(context.Background(), 49)
	if err != nil || a.Title() != "SCP-049" || a.Rating != "" || a.URL != "https://scp-wiki.wikidot.com/scp-049" {
		t.Fatalf("index failure fallback: %+v, %v", a, err)
	}

	_, err = c.Lookup(context.Background(), 4321)
	if err == nil || err.Error() != "SCP-4321 doesn't exist yet." {
		t.Fatalf("missing article: %v", err)
	}

	for _, n := range []int{0, 12345} {
		if _, err := c.Lookup(context.Background(), n); err == nil || !strings.Contains(err.Error(), "between 1 and 9999") {
			t.Errorf("Lookup(%d): %v", n, err)
		}
	}

	w.down = true
	if _, err := c.Lookup(context.Background(), 3000); !errors.Is(err, errSCPUnavailable) {
		t.Fatalf("outage: %v", err)
	}
}

func TestSCPEmbed(t *testing.T) {
	e := SCPEmbed(SCPArticle{Number: 3000, Name: "Anantashesha", Rating: "+3029", URL: "https://scp-wiki.wikidot.com/scp-3000"})
	if e.Title != "SCP-3000 — Anantashesha" || e.Description != "Rating: **+3029**" || e.Footer.Text != "SCP Foundation · CC BY-SA 3.0" || e.URL == "" {
		t.Fatalf("embed: %+v", e)
	}
	if e := SCPEmbed(SCPArticle{Number: 49, URL: "u"}); e.Title != "SCP-049" || e.Description != "" {
		t.Fatalf("bare embed: %+v", e)
	}
}

func TestSCPCommand(t *testing.T) {
	s := New("")
	fake, _ := newFakeSCP(t)
	s.scp = fake
	reg := cmd.NewRegistry(slog.New(slog.DiscardHandler), nil, "%")
	reg.Add(s.Commands()...)
	command, ok := reg.Lookup("scp")
	if !ok || !command.Heavy {
		t.Fatal("scp not registered as a heavy command")
	}

	c, rec := cmd.NewTestContext(reg, map[string]any{"number": 3000})
	if err := command.Run(c); err != nil {
		t.Fatal(err)
	}
	if e := rec.Messages[0].Embeds[0]; e.Title != "SCP-3000 — Anantashesha" || e.URL != "https://scp-wiki.wikidot.com/scp-3000" || e.Description != "Rating: **+3029**" {
		t.Fatalf("/scp 3000: %+v", e)
	}

	rec, err := cmd.RunPrefix(reg, command, "49")
	if err != nil || rec.Messages[0].Embeds[0].Title != "SCP-049" || rec.Messages[0].Embeds[0].URL != "https://scp-wiki.wikidot.com/scp-049" {
		t.Fatalf("%%scp 49: %v %+v", err, rec.Messages)
	}

	rec, err = cmd.RunPrefix(reg, command, "12345")
	if err != nil || !strings.Contains(rec.Messages[0].Content, "must be between 1 and 9999") {
		t.Fatalf("%%scp 12345: %v %+v", err, rec.Messages)
	}
}
