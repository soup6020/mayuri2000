package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const paginatorTTL = 15 * time.Minute

// Page is one page of a paginated reply.
type Page struct {
	Content string
	Embed   *discord.Embed
}

type pagination struct {
	owner   snowflake.ID
	pages   []Page
	expires time.Time
}

type paginators struct {
	mu    sync.Mutex
	next  atomic.Uint64
	items map[uint64]*pagination
}

var pager = &paginators{items: map[uint64]*pagination{}}

func (p *paginators) add(pg *pagination) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for id, item := range p.items {
		if now.After(item.expires) {
			delete(p.items, id)
		}
	}
	id := p.next.Add(1)
	p.items[id] = pg
	return id
}

func (p *paginators) get(id uint64) (*pagination, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pg, ok := p.items[id]
	if !ok || time.Now().After(pg.expires) {
		return nil, false
	}
	return pg, true
}

func pageMessage(id uint64, pages []Page, idx int) (string, []discord.Embed, []discord.LayoutComponent) {
	page := pages[idx]
	var embeds []discord.Embed
	if page.Embed != nil {
		e := *page.Embed
		if len(pages) > 1 {
			footer := fmt.Sprintf("Page %d/%d", idx+1, len(pages))
			if e.Footer != nil && e.Footer.Text != "" {
				footer = e.Footer.Text + " • " + footer
			}
			f := discord.EmbedFooter{Text: footer}
			if e.Footer != nil {
				f.IconURL = e.Footer.IconURL
			}
			e.Footer = &f
		}
		embeds = []discord.Embed{e}
	}
	content := page.Content
	if page.Embed == nil && len(pages) > 1 {
		content = fmt.Sprintf("%s\n-# %d/%d", content, idx+1, len(pages))
	}
	var comps []discord.LayoutComponent
	if len(pages) > 1 {
		prev := (idx - 1 + len(pages)) % len(pages)
		next := (idx + 1) % len(pages)
		// The direction suffix keeps custom IDs unique when prev == next
		// (two pages), which Discord would otherwise reject.
		comps = []discord.LayoutComponent{discord.NewActionRow(
			discord.NewSecondaryButton("◀", fmt.Sprintf("pg:%d:%d:p", id, prev)),
			discord.NewSecondaryButton("▶", fmt.Sprintf("pg:%d:%d:n", id, next)),
		)}
	}
	return content, embeds, comps
}

// Paginate sends pages with previous/next buttons usable by the invoker.
func (c *Context) Paginate(pages []Page) error {
	if len(pages) == 0 {
		return Errorf("Nothing found.")
	}
	var id uint64
	if len(pages) > 1 {
		id = pager.add(&pagination{owner: c.Author.ID, pages: pages, expires: time.Now().Add(paginatorTTL)})
	}
	content, embeds, comps := pageMessage(id, pages, 0)
	return c.Send(discord.MessageCreate{Content: content, Embeds: embeds, Components: comps})
}

func handlePageButton(e *events.ComponentInteractionCreate, data string) error {
	idStr, rest, _ := strings.Cut(data, ":")
	idxStr, _, _ := strings.Cut(rest, ":")
	id, _ := strconv.ParseUint(idStr, 10, 64)
	idx, _ := strconv.Atoi(idxStr)
	pg, ok := pager.get(id)
	if !ok {
		return e.UpdateMessage(discord.MessageUpdate{Components: &[]discord.LayoutComponent{}})
	}
	if e.User().ID != pg.owner {
		return e.CreateMessage(discord.MessageCreate{Content: "Only the person who ran the command can flip pages.", Flags: discord.MessageFlagEphemeral})
	}
	if idx < 0 || idx >= len(pg.pages) {
		idx = 0
	}
	content, embeds, comps := pageMessage(id, pg.pages, idx)
	return e.UpdateMessage(discord.MessageUpdate{Content: &content, Embeds: &embeds, Components: &comps})
}
