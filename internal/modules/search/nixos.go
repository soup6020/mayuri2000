package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/htmlmd"
)

const nixosSearchURL = "https://search.nixos.org"

// NixosConfig is the search backend configuration that search.nixos.org's
// frontend embeds in its JavaScript bundle.
type NixosConfig struct {
	SchemaVersion int
	Username      string
	Password      string
	// Channels maps channel IDs ("26.05", "unstable") to branches.
	Channels       map[string]string
	DefaultChannel string
}

// defaultNixosConfig is used when the frontend can't be scraped.
var defaultNixosConfig = NixosConfig{
	SchemaVersion:  51,
	Username:       "aWVSALXpZv",
	Password:       "X8gPHnzL52wFEekuxsfQ9cSh",
	Channels:       map[string]string{"26.05": "nixos-26.05", "unstable": "nixos-unstable"},
	DefaultChannel: "26.05",
}

var (
	bundleRe   = regexp.MustCompile(`src="(/static/js/index\.[0-9a-f]+\.js)"`)
	schemaRe   = regexp.MustCompile(`elasticsearchMappingSchemaVersion:parseInt\("(\d+)"\)`)
	userRe     = regexp.MustCompile(`elasticsearchUsername:"([^"]+)"`)
	passRe     = regexp.MustCompile(`elasticsearchPassword:"([^"]+)"`)
	channelsRe = regexp.MustCompile(`nixosChannels:JSON\.parse\('([^']+)'\)`)
)

// ParseNixosBundle extracts the backend configuration from the frontend JS.
func ParseNixosBundle(js []byte) (NixosConfig, error) {
	var cfg NixosConfig
	m := schemaRe.FindSubmatch(js)
	u := userRe.FindSubmatch(js)
	p := passRe.FindSubmatch(js)
	ch := channelsRe.FindSubmatch(js)
	if m == nil || u == nil || p == nil || ch == nil {
		return cfg, fmt.Errorf("search.nixos.org frontend format changed")
	}
	cfg.SchemaVersion, _ = strconv.Atoi(string(m[1]))
	cfg.Username, cfg.Password = string(u[1]), string(p[1])
	var channels struct {
		Channels []struct {
			ID     string `json:"id"`
			Branch string `json:"branch"`
		} `json:"channels"`
		Default string `json:"default"`
	}
	if err := json.Unmarshal(ch[1], &channels); err != nil {
		return cfg, err
	}
	cfg.Channels = map[string]string{}
	for _, c := range channels.Channels {
		cfg.Channels[c.ID] = c.Branch
	}
	cfg.DefaultChannel = channels.Default
	if cfg.SchemaVersion == 0 || len(cfg.Channels) == 0 || cfg.Channels[cfg.DefaultChannel] == "" {
		return cfg, fmt.Errorf("incomplete search.nixos.org config")
	}
	return cfg, nil
}

type nixosConfigCache struct {
	mu      sync.Mutex
	cfg     NixosConfig
	fetched time.Time
}

// config returns the backend config, re-scraping the frontend every few hours
// so schema bumps and new releases are picked up automatically.
func (n *nixosConfigCache) config(ctx context.Context) NixosConfig {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.fetched.IsZero() && time.Since(n.fetched) < 6*time.Hour {
		return n.cfg
	}
	cfg, err := fetchNixosConfig(ctx)
	if err != nil {
		if n.fetched.IsZero() {
			n.cfg = defaultNixosConfig
		}
		// Retry sooner after a failure.
		n.fetched = time.Now().Add(-5*time.Hour - 50*time.Minute)
		return n.cfg
	}
	n.cfg, n.fetched = cfg, time.Now()
	return cfg
}

func fetchNixosConfig(ctx context.Context) (NixosConfig, error) {
	page, err := get(ctx, nixosSearchURL+"/packages")
	if err != nil {
		return NixosConfig{}, err
	}
	m := bundleRe.FindSubmatch(page)
	if m == nil {
		return NixosConfig{}, fmt.Errorf("no frontend bundle found")
	}
	js, err := get(ctx, nixosSearchURL+string(m[1]))
	if err != nil {
		return NixosConfig{}, err
	}
	return ParseNixosBundle(js)
}

// NixPackage is a package document from the search backend.
type NixPackage struct {
	AttrName    string   `json:"package_attr_name"`
	Pname       string   `json:"package_pname"`
	Version     string   `json:"package_pversion"`
	Description string   `json:"package_description"`
	LongDesc    string   `json:"package_longDescription"`
	Homepage    []string `json:"package_homepage"`
	Programs    []string `json:"package_programs"`
	Platforms   []string `json:"package_platforms"`
	Position    string   `json:"package_position"`
	Licenses    []struct {
		FullName string `json:"fullName"`
		SpdxID   string `json:"spdxId"`
	} `json:"package_license"`
	Maintainers []struct {
		Name   string `json:"name"`
		GitHub string `json:"github"`
	} `json:"package_maintainers"`
}

// nixosFields are the weighted search fields per document type, mirroring
// the search.nixos.org frontend.
var nixosFields = map[string][]string{
	"package": {
		"package_attr_name^9", "package_attr_name.*^5.4",
		"package_programs^9", "package_programs.*^5.4",
		"package_pname^6", "package_pname.*^3.6",
		"package_description^1.3", "package_description.*^0.78",
		"package_longDescription^1", "package_longDescription.*^0.6",
	},
	"option": {
		"option_name^6", "option_name.*^3.6",
		"option_description^1", "option_description.*^0.6",
	},
}

// nixosQuery builds an Elasticsearch query for docType ("package" or
// "option"), boosting exact name matches so "firefox" beats
// "firefox-unwrapped".
func nixosQuery(docType, query string, size int) ([]byte, error) {
	nameField := docType + "_name"
	exact := []any{}
	if docType == "package" {
		nameField = "package_attr_name"
		exact = append(exact, map[string]any{"term": map[string]any{"package_pname": map[string]any{"value": query, "boost": 20, "case_insensitive": true}}})
	}
	exact = append(exact, map[string]any{"term": map[string]any{nameField: map[string]any{"value": query, "boost": 100, "case_insensitive": true}}})

	queries := []any{map[string]any{"multi_match": map[string]any{
		"type": "cross_fields", "query": query, "analyzer": "whitespace",
		"auto_generate_synonyms_phrase_query": false, "operator": "and", "fields": nixosFields[docType],
	}}}
	for _, word := range strings.Fields(query) {
		queries = append(queries, map[string]any{"wildcard": map[string]any{
			nameField: map[string]any{"value": "*" + word + "*", "case_insensitive": true},
		}})
	}
	return json.Marshal(map[string]any{
		"size": size,
		"query": map[string]any{"bool": map[string]any{
			"filter": []any{map[string]any{"term": map[string]any{"type": docType}}},
			"should": exact,
			"must":   []any{map[string]any{"dis_max": map[string]any{"tie_breaker": 0.7, "queries": queries}}},
		}},
	})
}

func parseHits[T any](data []byte) ([]T, error) {
	var out struct {
		Hits struct {
			Hits []struct {
				Source T `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out.Error) > 0 {
		return nil, fmt.Errorf("search backend error: %s", truncate(string(out.Error), 200))
	}
	docs := make([]T, len(out.Hits.Hits))
	for i, h := range out.Hits.Hits {
		docs[i] = h.Source
	}
	return docs, nil
}

// ParseNixosSearch extracts packages from a search response.
func ParseNixosSearch(data []byte) ([]NixPackage, error) { return parseHits[NixPackage](data) }

// ParseNixosOptions extracts options from a search response.
func ParseNixosOptions(data []byte) ([]NixOption, error) { return parseHits[NixOption](data) }

// NixOption is a NixOS option document from the search backend.
type NixOption struct {
	Name        string  `json:"option_name"`
	Description string  `json:"option_description"`
	Type        string  `json:"option_type"`
	Default     *string `json:"option_default"`
	Example     *string `json:"option_example"`
	Source      string  `json:"option_source"`
}

// nixosSearch runs a search of docType against the channel's index.
func (s *Searcher) nixosSearch(ctx context.Context, cfg NixosConfig, channel, docType, query string) ([]byte, error) {
	body, err := nixosQuery(docType, query, 10)
	if err != nil {
		return nil, err
	}
	index := fmt.Sprintf("latest-%d-%s", cfg.SchemaVersion, cfg.Channels[channel])
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, nixosSearchURL+"/backend/"+index+"/_search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	return do(req)
}

// platformSummary condenses the platform list, e.g. "Linux, macOS".
func platformSummary(platforms []string) string {
	has := map[string]bool{}
	for _, p := range platforms {
		switch {
		case strings.HasSuffix(p, "-linux"):
			has["Linux"] = true
		case strings.HasSuffix(p, "-darwin"):
			has["macOS"] = true
		case strings.HasSuffix(p, "-freebsd"):
			has["FreeBSD"] = true
		case strings.HasSuffix(p, "-windows") || strings.HasSuffix(p, "-cygwin"):
			has["Windows"] = true
		}
	}
	var out []string
	for _, os := range []string{"Linux", "macOS", "FreeBSD", "Windows"} {
		if has[os] {
			out = append(out, os)
		}
	}
	if len(out) == 0 {
		return "Unknown"
	}
	return strings.Join(out, ", ")
}

func nixPackageEmbed(p NixPackage, channel, branch, query string) discord.Embed {
	show := fmt.Sprintf("%s/packages?channel=%s&show=%s&query=%s", nixosSearchURL, url.QueryEscape(channel), url.QueryEscape(p.AttrName), url.QueryEscape(query))
	desc := p.Description
	if desc == "" {
		desc = "*No description*"
	}
	desc += fmt.Sprintf("\n```sh\nnix shell nixpkgs#%s\n```", p.AttrName)
	e := discord.Embed{
		Title:       truncate(fmt.Sprintf("%s %s", p.AttrName, p.Version), 256),
		URL:         show,
		Description: truncate(desc, 4000),
		Color:       0x7EBAE4,
		Footer:      &discord.EmbedFooter{Text: "NixOS " + channel},
	}
	if len(p.Homepage) > 0 {
		e.Fields = append(e.Fields, inline("Homepage", truncate(p.Homepage[0], 1024)))
	}
	if len(p.Licenses) > 0 {
		var names []string
		for _, l := range p.Licenses {
			name := l.SpdxID
			if name == "" {
				name = l.FullName
			}
			names = append(names, name)
		}
		e.Fields = append(e.Fields, inline("License", truncate(strings.Join(names, ", "), 1024)))
	}
	e.Fields = append(e.Fields, inline("Platforms", platformSummary(p.Platforms)))
	if len(p.Programs) > 0 {
		progs := p.Programs
		sort.Strings(progs)
		list := "`" + strings.Join(progs[:min(8, len(progs))], "`, `") + "`"
		if len(progs) > 8 {
			list += fmt.Sprintf(" and %d more", len(progs)-8)
		}
		e.Fields = append(e.Fields, inline("Programs", list))
	}
	if len(p.Maintainers) > 0 {
		var names []string
		for _, m := range p.Maintainers[:min(5, len(p.Maintainers))] {
			if m.GitHub != "" {
				names = append(names, fmt.Sprintf("[%s](https://github.com/%s)", m.Name, m.GitHub))
			} else {
				names = append(names, m.Name)
			}
		}
		if len(p.Maintainers) > 5 {
			names = append(names, fmt.Sprintf("and %d more", len(p.Maintainers)-5))
		}
		e.Fields = append(e.Fields, inline("Maintainers", truncate(strings.Join(names, ", "), 1024)))
	}
	if p.Position != "" {
		e.Fields = append(e.Fields, inline("Source", nixpkgsLink(branch, p.Position)))
	}
	return e
}

// nixpkgsLink links a "path[:line]" position to nixpkgs on GitHub.
func nixpkgsLink(branch, position string) string {
	file, line, _ := strings.Cut(position, ":")
	ref := branch
	if branch == "nixos-unstable" {
		ref = "master"
	}
	src := fmt.Sprintf("https://github.com/NixOS/nixpkgs/blob/%s/%s", ref, file)
	if line != "" {
		src += "#L" + line
	}
	return fmt.Sprintf("[%s](%s)", truncate(position, 200), src)
}

func inline(name, value string) discord.EmbedField {
	return discord.EmbedField{Name: name, Value: value, Inline: cmd.Ptr(true)}
}

// nixosChannel picks the channel from the command's "channel" option.
func (s *Searcher) nixosChannel(c *cmd.Context) (NixosConfig, string) {
	cfg := s.nixos.config(c)
	channel := cfg.DefaultChannel
	if c.StringOr("channel", "stable") == "unstable" {
		if _, ok := cfg.Channels["unstable"]; ok {
			channel = "unstable"
		}
	}
	return cfg, channel
}

func (s *Searcher) nixpkgs(c *cmd.Context) error {
	query := strings.TrimSpace(c.String("query"))
	cfg, channel := s.nixosChannel(c)
	data, err := s.nixosSearch(c, cfg, channel, "package", query)
	if err != nil {
		return cmd.Errorf("search.nixos.org isn't responding right now.")
	}
	pkgs, err := ParseNixosSearch(data)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return cmd.Errorf("No packages found for **%s** in NixOS %s.", query, channel)
	}
	pages := make([]cmd.Page, len(pkgs))
	for i, p := range pkgs {
		e := nixPackageEmbed(p, channel, cfg.Channels[channel], query)
		pages[i] = cmd.Page{Embed: &e}
	}
	return c.Paginate(pages)
}

func nixCodeBlock(value string, limit int) string {
	return "```nix\n" + truncate(strings.TrimSpace(value), limit-12) + "\n```"
}

func nixOptionEmbed(o NixOption, channel, branch, query string) discord.Embed {
	show := fmt.Sprintf("%s/options?channel=%s&show=%s&query=%s", nixosSearchURL, url.QueryEscape(channel), url.QueryEscape(o.Name), url.QueryEscape(query))
	desc := htmlmd.Convert(o.Description)
	if desc == "" {
		desc = "*No description*"
	}
	e := discord.Embed{
		Title:       truncate(o.Name, 256),
		URL:         show,
		Description: truncate(desc, 3000),
		Color:       0x7EBAE4,
		Footer:      &discord.EmbedFooter{Text: "NixOS " + channel},
		Fields:      []discord.EmbedField{{Name: "Type", Value: truncate(o.Type, 1024)}},
	}
	if o.Default != nil && *o.Default != "" {
		e.Fields = append(e.Fields, discord.EmbedField{Name: "Default", Value: nixCodeBlock(*o.Default, 1024)})
	}
	if o.Example != nil && *o.Example != "" {
		e.Fields = append(e.Fields, discord.EmbedField{Name: "Example", Value: nixCodeBlock(*o.Example, 1024)})
	}
	if o.Source != "" {
		e.Fields = append(e.Fields, discord.EmbedField{Name: "Declared in", Value: nixpkgsLink(branch, o.Source)})
	}
	return e
}

func (s *Searcher) nixosOptions(c *cmd.Context) error {
	query := strings.TrimSpace(c.String("query"))
	cfg, channel := s.nixosChannel(c)
	data, err := s.nixosSearch(c, cfg, channel, "option", query)
	if err != nil {
		return cmd.Errorf("search.nixos.org isn't responding right now.")
	}
	opts, err := ParseNixosOptions(data)
	if err != nil {
		return err
	}
	if len(opts) == 0 {
		return cmd.Errorf("No NixOS options found for **%s** in NixOS %s.", query, channel)
	}
	pages := make([]cmd.Page, len(opts))
	for i, o := range opts {
		e := nixOptionEmbed(o, channel, cfg.Channels[channel], query)
		pages[i] = cmd.Page{Embed: &e}
	}
	return c.Paginate(pages)
}
