package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
)

const wikiREST = "https://en.wikipedia.org/api/rest_v1"

var wikiFooter = &discord.EmbedFooter{
	Text:    "Information provided by Wikimedia",
	IconURL: "https://upload.wikimedia.org/wikipedia/commons/thumb/5/53/Wikimedia-logo.png/600px-Wikimedia-logo.png",
}

// WikiSummary is a page summary from Wikipedia's REST API.
type WikiSummary struct {
	Type           string `json:"type"`
	Title          string `json:"title"`
	NormalizedName string `json:"normalizedtitle"`
	Titles         struct {
		Normalized string `json:"normalized"`
	} `json:"titles"`
	Description string    `json:"description"`
	Extract     string    `json:"extract"`
	Timestamp   time.Time `json:"timestamp"`
	Views       int       `json:"views"`
	ContentURLs struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
	Thumbnail *struct {
		Source string `json:"source"`
	} `json:"thumbnail"`
	OriginalImage *struct {
		Source string `json:"source"`
	} `json:"originalimage"`
}

// Name is the human-readable page title.
func (s WikiSummary) Name() string {
	switch {
	case s.Titles.Normalized != "":
		return s.Titles.Normalized
	case s.NormalizedName != "":
		return s.NormalizedName
	}
	return strings.ReplaceAll(s.Title, "_", " ")
}

// WikiFeatured is the subset of the daily featured feed we display.
type WikiFeatured struct {
	TFA   *WikiSummary `json:"tfa"`
	Image *struct {
		Title       string `json:"title"`
		Description struct {
			Text string `json:"text"`
		} `json:"description"`
		Artist struct {
			Text string `json:"text"`
		} `json:"artist"`
		License struct {
			Type string `json:"type"`
		} `json:"license"`
		Thumbnail struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		FilePage string `json:"file_page"`
	} `json:"image"`
	MostRead *struct {
		Articles []WikiSummary `json:"articles"`
	} `json:"mostread"`
}

func ParseWikiFeatured(data []byte) (*WikiFeatured, error) {
	var f WikiFeatured
	return &f, json.Unmarshal(data, &f)
}

func ParseWikiSummary(data []byte) (*WikiSummary, error) {
	var s WikiSummary
	return &s, json.Unmarshal(data, &s)
}

// wikiSummaryEmbed renders a page summary with a heading like "Random article".
func wikiSummaryEmbed(s WikiSummary, heading string) discord.Embed {
	desc := s.Extract
	if r := []rune(desc); len(r) > maxWikiDesc {
		desc = strings.TrimSpace(string(r[:maxWikiDesc])) + "…"
	}
	if s.Description != "" {
		desc = "*" + s.Description + "*\n\n" + desc
	}
	desc += fmt.Sprintf("\n\n[Read the article](%s)", s.ContentURLs.Desktop.Page)
	e := discord.Embed{
		Author:      &discord.EmbedAuthor{Name: heading},
		Title:       truncate(s.Name(), 256),
		URL:         s.ContentURLs.Desktop.Page,
		Description: desc,
		Color:       0x3498DB,
		Footer:      wikiFooter,
	}
	switch {
	case s.OriginalImage != nil && s.OriginalImage.Source != "":
		e.Image = &discord.EmbedResource{URL: s.OriginalImage.Source}
	case s.Thumbnail != nil && s.Thumbnail.Source != "":
		e.Image = &discord.EmbedResource{URL: s.Thumbnail.Source}
	}
	if !s.Timestamp.IsZero() {
		t := s.Timestamp
		e.Timestamp = &t
	}
	return e
}

// featuredPages turns the daily feed into pages: featured article, picture
// of the day, and most read articles.
func featuredPages(f *WikiFeatured, day time.Time) []cmd.Page {
	date := day.Format("2 January 2006")
	var pages []cmd.Page
	if f.TFA != nil {
		e := wikiSummaryEmbed(*f.TFA, "⭐ Featured article · "+date)
		pages = append(pages, cmd.Page{Embed: &e})
	}
	if img := f.Image; img != nil && img.Thumbnail.Source != "" {
		desc := img.Description.Text
		if img.Artist.Text != "" {
			desc += "\n\n📷 " + img.Artist.Text
			if img.License.Type != "" {
				desc += " · " + img.License.Type
			}
		}
		e := discord.Embed{
			Author:      &discord.EmbedAuthor{Name: "🖼️ Picture of the day · " + date},
			Title:       truncate(strings.TrimPrefix(img.Title, "File:"), 256),
			URL:         img.FilePage,
			Description: truncate(desc, 2000),
			Image:       &discord.EmbedResource{URL: img.Thumbnail.Source},
			Color:       0x3498DB,
			Footer:      wikiFooter,
		}
		pages = append(pages, cmd.Page{Embed: &e})
	}
	if f.MostRead != nil && len(f.MostRead.Articles) > 0 {
		var b strings.Builder
		for i, a := range f.MostRead.Articles[:min(10, len(f.MostRead.Articles))] {
			fmt.Fprintf(&b, "%d. [%s](%s) · %s views\n", i+1, a.Name(), a.ContentURLs.Desktop.Page, formatCount(a.Views))
		}
		e := discord.Embed{
			Author:      &discord.EmbedAuthor{Name: "📈 Most read · " + day.AddDate(0, 0, -1).Format("2 January 2006")},
			Title:       "Most read articles",
			Description: b.String(),
			Color:       0x3498DB,
			Footer:      wikiFooter,
		}
		pages = append(pages, cmd.Page{Embed: &e})
	}
	return pages
}

// formatCount renders 421359 as "421,359".
func formatCount(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// parseDay accepts YYYY-MM-DD, "today" or "yesterday".
func parseDay(s string, now time.Time) (time.Time, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	}
	d, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, cmd.Errorf("Dates look like `2024-05-17`.")
	}
	if d.After(today) {
		return time.Time{}, cmd.Errorf("That date is in the future.")
	}
	if d.Before(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return time.Time{}, cmd.Errorf("The featured feed only goes back to 2016.")
	}
	return d, nil
}

func (s *Searcher) wikiFeatured(c *cmd.Context) error {
	day, err := parseDay(c.String("date"), time.Now().UTC())
	if err != nil {
		return err
	}
	data, err := get(c, fmt.Sprintf("%s/feed/featured/%s", wikiREST, day.Format("2006/01/02")))
	if err != nil {
		return cmd.Errorf("Wikipedia isn't responding right now.")
	}
	feed, err := ParseWikiFeatured(data)
	if err != nil {
		return err
	}
	pages := featuredPages(feed, day)
	if len(pages) == 0 {
		return cmd.Errorf("Wikipedia has nothing featured for %s yet.", day.Format("2 January 2006"))
	}
	return c.Paginate(pages)
}

// randomArticle fetches random articles until it finds a regular page
// (skipping disambiguation pages).
func randomArticle(ctx context.Context) (*WikiSummary, error) {
	for range 3 {
		data, err := get(ctx, wikiREST+"/page/random/summary")
		if err != nil {
			return nil, err
		}
		sum, err := ParseWikiSummary(data)
		if err != nil {
			return nil, err
		}
		if sum.Type == "standard" || sum.Type == "" {
			return sum, nil
		}
	}
	return nil, fmt.Errorf("only found disambiguation pages")
}

func rerollButton(owner snowflake.ID) []discord.LayoutComponent {
	return []discord.LayoutComponent{discord.NewActionRow(
		discord.NewSecondaryButton("🎲 Another", "wikirand:"+owner.String()),
	)}
}

func (s *Searcher) wikiRandom(c *cmd.Context) error {
	sum, err := randomArticle(c)
	if err != nil {
		return cmd.Errorf("Wikipedia isn't responding right now.")
	}
	return c.Send(discord.MessageCreate{
		Embeds:     []discord.Embed{wikiSummaryEmbed(*sum, "🎲 Random article")},
		Components: rerollButton(c.Author.ID),
	})
}

// Components returns the search module's button handlers.
func (s *Searcher) Components() map[string]cmd.ComponentHandler {
	return map[string]cmd.ComponentHandler{"wikirand": s.rerollRandom}
}

// rerollRandom replaces a random article with another one.
func (s *Searcher) rerollRandom(e *events.ComponentInteractionCreate, data string) error {
	owner, _ := snowflake.Parse(data)
	if e.User().ID != owner {
		return e.CreateMessage(discord.MessageCreate{Content: "Only the person who ran the command can reroll it. Try `/wikirandom` yourself!", Flags: discord.MessageFlagEphemeral})
	}
	if err := e.DeferUpdateMessage(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sum, err := randomArticle(ctx)
	if err != nil {
		return err
	}
	embeds := []discord.Embed{wikiSummaryEmbed(*sum, "🎲 Random article")}
	_, err = e.Client().Rest.UpdateInteractionResponse(e.ApplicationID(), e.Token(), discord.MessageUpdate{Embeds: &embeds})
	return err
}
