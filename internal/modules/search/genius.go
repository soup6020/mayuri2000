package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"golang.org/x/net/html"

	"mayuri2000/internal/cmd"
)

type GeniusSong struct {
	FullTitle string `json:"full_title"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	ArtURL    string `json:"song_art_image_thumbnail_url"`
	Artist    struct {
		Name string `json:"name"`
	} `json:"primary_artist"`
}

// ParseGeniusSearch reads songs from either the public search endpoint
// (response.sections[].hits) or the official API (response.hits).
func ParseGeniusSearch(data []byte) ([]GeniusSong, error) {
	type hit struct {
		Type   string     `json:"type"`
		Result GeniusSong `json:"result"`
	}
	var out struct {
		Response struct {
			Hits     []hit `json:"hits"`
			Sections []struct {
				Hits []hit `json:"hits"`
			} `json:"sections"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	hits := out.Response.Hits
	for _, s := range out.Response.Sections {
		hits = append(hits, s.Hits...)
	}
	var songs []GeniusSong
	for _, h := range hits {
		if (h.Type == "" || h.Type == "song") && h.Result.URL != "" {
			songs = append(songs, h.Result)
		}
	}
	return songs, nil
}

func (s *Searcher) geniusSearch(ctx context.Context, query string) ([]GeniusSong, error) {
	if s.geniusToken != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.genius.com/search?q="+url.QueryEscape(query), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+s.geniusToken)
		data, err := do(req)
		if err == nil {
			return ParseGeniusSearch(data)
		}
		// Fall through to the public endpoint if the token is rejected.
	}
	data, err := getBrowser(ctx, "https://genius.com/api/search/song?per_page=5&q="+url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	return ParseGeniusSearch(data)
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var blankLines = regexp.MustCompile(`\n{3,}`)

// ExtractLyrics pulls the lyrics text out of a Genius song page.
func ExtractLyrics(page []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var text func(*html.Node)
	text = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			return
		case html.ElementNode:
			if attr(n, "data-exclude-from-selection") == "true" {
				return
			}
			switch n.Data {
			case "br":
				b.WriteByte('\n')
				return
			case "script", "style", "button", "svg":
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			text(c)
		}
	}
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && attr(n, "data-lyrics-container") == "true" {
			text(n)
			b.WriteByte('\n')
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)

	lyrics := strings.TrimSpace(blankLines.ReplaceAllString(b.String(), "\n\n"))
	// Put a blank line before section headers like [Chorus] for readability.
	lines := strings.Split(lyrics, "\n")
	var out []string
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if i > 0 && strings.HasPrefix(l, "[") && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, l)
	}
	lyrics = strings.Join(out, "\n")
	if lyrics == "" {
		return "", fmt.Errorf("no lyrics found on page")
	}
	return lyrics, nil
}

// chunkLines splits text into chunks of at most n runes on line boundaries.
func chunkLines(text string, n int) []string {
	var chunks []string
	var cur strings.Builder
	curLen := 0
	for _, line := range strings.Split(text, "\n") {
		l := len([]rune(line)) + 1
		if curLen+l > n && curLen > 0 {
			chunks = append(chunks, strings.TrimSpace(cur.String()))
			cur.Reset()
			curLen = 0
		}
		if l > n {
			line = truncate(line, n-1)
			l = n
		}
		cur.WriteString(line + "\n")
		curLen += l
	}
	if strings.TrimSpace(cur.String()) != "" {
		chunks = append(chunks, strings.TrimSpace(cur.String()))
	}
	return chunks
}

func (s *Searcher) lyrics(c *cmd.Context) error {
	query := c.String("query")
	songs, err := s.geniusSearch(c, query)
	if err != nil {
		return cmd.Errorf("Genius isn't responding right now.")
	}
	if len(songs) == 0 {
		return cmd.Errorf("No songs found on Genius for **%s**.", query)
	}
	song := songs[0]
	page, err := getBrowser(c, song.URL)
	if err != nil {
		return cmd.Errorf("I couldn't load the lyrics page for **%s**.", song.FullTitle)
	}
	lyrics, err := ExtractLyrics(page)
	if err != nil {
		return cmd.Errorf("I found [%s](<%s>) but couldn't read its lyrics.", song.FullTitle, song.URL)
	}
	chunks := chunkLines(lyrics, 3500)
	pages := make([]cmd.Page, len(chunks))
	for i, chunk := range chunks {
		e := discord.Embed{
			Title:       truncate(song.Title+" — "+song.Artist.Name, 256),
			URL:         song.URL,
			Description: chunk,
			Color:       0xFFFF64,
			Footer:      &discord.EmbedFooter{Text: "Lyrics from Genius"},
		}
		if song.ArtURL != "" {
			e.Thumbnail = &discord.EmbedResource{URL: song.ArtURL}
		}
		pages[i] = cmd.Page{Embed: &e}
	}
	return c.Paginate(pages)
}
