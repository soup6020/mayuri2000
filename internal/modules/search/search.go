// Package search provides Urban Dictionary, Wikipedia and YouTube lookups.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"

	"mayuri2000/internal/cmd"
)

const category = "Search"

const userAgent = "Mayuri2000/1.0 (Discord bot; https://github.com/disgoorg/disgo)"

var httpClient = &http.Client{Timeout: 15 * time.Second}

func nixosChannelOpt() cmd.Option {
	return cmd.Option{Name: "channel", Description: "NixOS channel (default: latest stable)", Type: cmd.String, SlashOnly: true, Choices: []cmd.Choice{
		{Name: "stable", Value: "stable"}, {Name: "unstable", Value: "unstable"},
	}}
}

func queryOpt(desc string) cmd.Option {
	return cmd.Option{Name: "query", Description: desc, Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(200)}
}

// Searcher holds configuration for the search commands.
type Searcher struct {
	geniusToken string
	nixos       nixosConfigCache
	frog        frogTips
}

func New(geniusToken string) *Searcher { return &Searcher{geniusToken: geniusToken} }

func (s *Searcher) Commands() []*cmd.Command {
	return []*cmd.Command{
		{
			Name: "lyrics", Aliases: []string{"genius"}, Category: category,
			Description: "Look up song lyrics on Genius",
			Options:     []cmd.Option{queryOpt("Song (and artist) to search for")},
			Heavy:       true,
			Run:         s.lyrics,
		},
		{
			Name: "nixpkgs", Aliases: []string{"nix", "nixos", "nixsearch"}, Category: category,
			Description: "Search NixOS packages on search.nixos.org",
			Options: []cmd.Option{
				{Name: "query", Description: "Package name, program or description", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(100)},
				nixosChannelOpt(),
			},
			Run: s.nixpkgs,
		},
		{
			Name: "nixoption", Aliases: []string{"nixopt", "nixopts", "nixoptions", "option"}, Category: category,
			Description: "Search NixOS configuration options on search.nixos.org",
			Options: []cmd.Option{
				{Name: "query", Description: "Option name or keywords, e.g. services.openssh", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(100)},
				nixosChannelOpt(),
			},
			Run: s.nixosOptions,
		},
		{
			Name: "frogtip", Aliases: []string{"frog", "frogtips"}, Category: "Fun",
			Description: "Get a random tip for operating your frog from frog.tips",
			Run:         s.frogTip,
		},
		{
			Name: "urban", Aliases: []string{"ud", "urbandictionary"}, Category: category,
			Description: "Look up a term on Urban Dictionary",
			Options:     []cmd.Option{queryOpt("Term to look up")},
			Run:         urban,
		},
		{
			Name: "wikipedia", Aliases: []string{"wiki"}, Category: category,
			Description: "Look something up on Wikipedia",
			Options:     []cmd.Option{queryOpt("What to search for")},
			Run:         wikipedia,
		},
		{
			Name: "wikifeatured", Aliases: []string{"featured", "tfa", "potd"}, Category: category,
			Description: "Wikipedia's featured article, picture of the day and most read",
			Options: []cmd.Option{
				{Name: "date", Description: "Day to show, e.g. 2024-05-17 (default: today, UTC)", Type: cmd.String, MaxLength: cmd.Ptr(10)},
			},
			Run: s.wikiFeatured,
		},
		{
			Name: "wikirandom", Aliases: []string{"randomwiki", "rwiki"}, Category: category,
			Description: "Show a random Wikipedia article",
			Run:         s.wikiRandom,
		},
		{
			Name: "youtube", Aliases: []string{"yt"}, Category: category,
			Description: "Search YouTube and post the top result",
			Options:     []cmd.Option{queryOpt("What to search for")},
			Run: func(c *cmd.Context) error {
				res, err := YouTube(c, c.String("query"))
				if err != nil {
					return err
				}
				return c.Reply(res[0])
			},
		},
		{
			Name: "ytsearch", Category: category,
			Description: "Search YouTube and browse the results",
			Options:     []cmd.Option{queryOpt("What to search for")},
			Run: func(c *cmd.Context) error {
				res, err := YouTube(c, c.String("query"))
				if err != nil {
					return err
				}
				pages := make([]cmd.Page, len(res))
				for i, u := range res {
					pages[i] = cmd.Page{Content: u}
				}
				return c.Paginate(pages)
			},
		},
	}
}

const browserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

func get(ctx context.Context, u string) ([]byte, error) {
	return getAs(ctx, u, userAgent)
}

// getBrowser fetches with a browser user agent for sites that block bots.
func getBrowser(ctx context.Context, u string) ([]byte, error) {
	return getAs(ctx, u, browserAgent)
}

func getAs(ctx context.Context, u, agent string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agent)
	return do(req)
}

func do(req *http.Request) ([]byte, error) {
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", req.URL.Host, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// --- Urban Dictionary ---

type UrbanDefinition struct {
	Word       string    `json:"word"`
	Definition string    `json:"definition"`
	Example    string    `json:"example"`
	Author     string    `json:"author"`
	Permalink  string    `json:"permalink"`
	ThumbsUp   int       `json:"thumbs_up"`
	ThumbsDown int       `json:"thumbs_down"`
	WrittenOn  time.Time `json:"written_on"`
}

func ParseUrban(data []byte) ([]UrbanDefinition, error) {
	var out struct {
		List []UrbanDefinition `json:"list"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

var urbanLinkRe = regexp.MustCompile(`\[([^\[\]]+)\]`)

// linkUrban turns Urban Dictionary's [term] references into markdown links.
func linkUrban(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	return urbanLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		term := m[1 : len(m)-1]
		return fmt.Sprintf("[%s](https://www.urbandictionary.com/define.php?term=%s)", term, url.QueryEscape(term))
	})
}

// fitLinked links terms, falling back to plain text if the links would
// push it past n characters.
func fitLinked(s string, n int) string {
	if l := linkUrban(s); len([]rune(l)) <= n {
		return l
	}
	return truncate(strings.NewReplacer("[", "", "]", "").Replace(s), n)
}

func urban(c *cmd.Context) error {
	term := c.String("query")
	data, err := get(c, "https://api.urbandictionary.com/v0/define?term="+url.QueryEscape(term))
	if err != nil {
		return cmd.Errorf("Urban Dictionary isn't responding right now.")
	}
	defs, err := ParseUrban(data)
	if err != nil {
		return err
	}
	if len(defs) == 0 {
		return cmd.Errorf("No Urban Dictionary entries for **%s**.", term)
	}
	sort.SliceStable(defs, func(i, j int) bool {
		return defs[i].ThumbsUp-defs[i].ThumbsDown > defs[j].ThumbsUp-defs[j].ThumbsDown
	})
	pages := make([]cmd.Page, len(defs))
	for i, d := range defs {
		e := discord.Embed{
			Title:       truncate(d.Word, 250),
			URL:         d.Permalink,
			Description: fitLinked(d.Definition, 4000),
			Color:       0x1D2439,
			Footer:      &discord.EmbedFooter{Text: fmt.Sprintf("👍 %d  👎 %d  •  by %s", d.ThumbsUp, d.ThumbsDown, truncate(d.Author, 60))},
		}
		if strings.TrimSpace(d.Example) != "" {
			e.Fields = []discord.EmbedField{{Name: "Example", Value: fitLinked("*"+strings.TrimSpace(d.Example)+"*", 1024)}}
		}
		if !d.WrittenOn.IsZero() {
			t := d.WrittenOn
			e.Timestamp = &t
		}
		pages[i] = cmd.Page{Embed: &e}
	}
	return c.Paginate(pages)
}

// --- Wikipedia ---

const (
	disambiguationCat = "Category:All disambiguation pages"
	maxWikiDesc       = 1000
)

var (
	wikiWhitespace = regexp.MustCompile(`[\n\s]{4,}`)
	wikiNewlines   = regexp.MustCompile(`\n+`)
)

type WikiPage struct {
	Index      int    `json:"index"`
	Title      string `json:"title"`
	Extract    string `json:"extract"`
	FullURL    string `json:"fullurl"`
	Categories []struct {
		Title string `json:"title"`
	} `json:"categories"`
	Original *struct {
		Source string `json:"source"`
	} `json:"original"`
	Revisions []struct {
		Timestamp time.Time `json:"timestamp"`
	} `json:"revisions"`
}

func wikiURL(query string) string {
	tokens := strings.Fields(query)
	v := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"generator":     {"search"},
		"redirects":     {"1"},
		"prop":          {"extracts|info|pageimages|revisions|categories"},
		"gsrsearch":     {"intitle:" + strings.Join(tokens, " intitle:")},
		"exintro":       {"1"},
		"explaintext":   {"1"},
		"inprop":        {"url"},
		"piprop":        {"original"},
		"rvprop":        {"timestamp"},
		"clcategories":  {disambiguationCat},
	}
	return "https://en.wikipedia.org/w/api.php?" + v.Encode()
}

// ParseWikipedia returns non-disambiguation pages in search order.
func ParseWikipedia(data []byte) ([]WikiPage, error) {
	var out struct {
		Query struct {
			Pages []WikiPage `json:"pages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	pages := out.Query.Pages
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Index < pages[j].Index })
	var kept []WikiPage
	for _, p := range pages {
		if len(p.Categories) > 0 && p.Categories[0].Title == disambiguationCat {
			continue
		}
		kept = append(kept, p)
	}
	return kept, nil
}

func wikiEmbed(p WikiPage) discord.Embed {
	desc := strings.TrimSpace(p.Extract)
	cut := false
	if loc := wikiWhitespace.FindStringIndex(desc); loc != nil && loc[0] > 0 {
		desc = strings.TrimSpace(desc[:loc[0]])
		cut = true
	}
	desc = wikiNewlines.ReplaceAllString(desc, "\n\n")
	if r := []rune(desc); len(r) > maxWikiDesc || cut {
		if len(r) > maxWikiDesc {
			desc = strings.TrimSpace(string(r[:maxWikiDesc]))
		}
		desc += fmt.Sprintf("... [(read more)](%s)", p.FullURL)
	}
	e := discord.Embed{
		Title:       "Wikipedia: " + p.Title,
		Description: desc,
		URL:         p.FullURL,
		Color:       0x3498DB,
		Footer: &discord.EmbedFooter{
			Text:    "Information provided by Wikimedia",
			IconURL: "https://upload.wikimedia.org/wikipedia/commons/thumb/5/53/Wikimedia-logo.png/600px-Wikimedia-logo.png",
		},
	}
	if p.Original != nil && p.Original.Source != "" {
		e.Image = &discord.EmbedResource{URL: p.Original.Source}
	}
	if len(p.Revisions) > 0 && !p.Revisions[0].Timestamp.IsZero() {
		t := p.Revisions[0].Timestamp
		e.Timestamp = &t
		e.Footer.Text += "\nArticle last updated"
	}
	return e
}

func wikipedia(c *cmd.Context) error {
	query := c.String("query")
	data, err := get(c, wikiURL(query))
	if err != nil {
		return cmd.Errorf("Wikipedia isn't responding right now.")
	}
	pages, err := ParseWikipedia(data)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return cmd.Errorf("I'm sorry, I couldn't find \"%s\" on Wikipedia.", query)
	}
	out := make([]cmd.Page, len(pages))
	for i, p := range pages {
		e := wikiEmbed(p)
		out[i] = cmd.Page{Embed: &e}
	}
	return c.Paginate(out)
}

// --- YouTube ---

var videoIDRe = regexp.MustCompile(`"videoId":"([A-Za-z0-9_-]{11})"`)

// ParseYouTube extracts unique video URLs from a results page.
func ParseYouTube(page []byte) []string {
	var urls []string
	seen := map[string]bool{}
	for _, m := range videoIDRe.FindAllSubmatch(page, -1) {
		id := string(m[1])
		if !seen[id] {
			seen[id] = true
			urls = append(urls, "https://www.youtube.com/watch?v="+id)
		}
	}
	return urls
}

// YouTube scrapes the search results page, like the old YouTube cog.
func YouTube(ctx context.Context, query string) ([]string, error) {
	data, err := get(ctx, "https://www.youtube.com/results?search_query="+url.QueryEscape(query))
	if err != nil {
		return nil, cmd.Errorf("YouTube isn't responding right now.")
	}
	res := ParseYouTube(data)
	if len(res) == 0 {
		return nil, cmd.Errorf("Nothing found on YouTube for **%s**.", query)
	}
	if len(res) > 25 {
		res = res[:25]
	}
	return res, nil
}
