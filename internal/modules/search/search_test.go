package search

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseUrban(t *testing.T) {
	data, err := os.ReadFile("testdata/urban.json")
	if err != nil {
		t.Fatal(err)
	}
	defs, err := ParseUrban(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) == 0 || !strings.EqualFold(defs[0].Word, "yeet") || defs[0].Definition == "" {
		t.Fatalf("unexpected definitions: %+v", defs)
	}
}

func TestLinkUrban(t *testing.T) {
	got := linkUrban("to [throw] something [really hard]")
	want := "to [throw](https://www.urbandictionary.com/define.php?term=throw) something [really hard](https://www.urbandictionary.com/define.php?term=really+hard)"
	if got != want {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("[word] ", 200)
	if out := fitLinked(long, 300); len([]rune(out)) > 300 || strings.Contains(out, "[") {
		t.Fatalf("fitLinked did not fall back to plain text: %d %q", len(out), out[:40])
	}
}

func TestParseWikipedia(t *testing.T) {
	data, err := os.ReadFile("testdata/wikipedia.json")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := ParseWikipedia(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no pages")
	}
	for i, p := range pages {
		if p.Title == "Mercury" {
			t.Fatal("disambiguation page was not filtered")
		}
		if i > 0 && pages[i-1].Index > p.Index {
			t.Fatal("pages not sorted by index")
		}
	}
	e := wikiEmbed(pages[0])
	if !strings.HasPrefix(e.Title, "Wikipedia: ") || e.URL == "" || len([]rune(e.Description)) > maxWikiDesc+200 {
		t.Fatalf("bad embed: %+v", e)
	}
}

func TestWikiURL(t *testing.T) {
	u := wikiURL("freddie mercury")
	if !strings.Contains(u, "gsrsearch=intitle%3Afreddie+intitle%3Amercury") {
		t.Fatalf("unexpected search param in %s", u)
	}
}

func TestParseYouTube(t *testing.T) {
	data, err := os.ReadFile("testdata/youtube.html")
	if err != nil {
		t.Fatal(err)
	}
	urls := ParseYouTube(data)
	if len(urls) == 0 {
		t.Fatal("no results")
	}
	seen := map[string]bool{}
	for _, u := range urls {
		if seen[u] {
			t.Fatalf("duplicate %s", u)
		}
		seen[u] = true
		if !strings.HasPrefix(u, "https://www.youtube.com/watch?v=") || len(u) != len("https://www.youtube.com/watch?v=")+11 {
			t.Fatalf("bad url %s", u)
		}
	}
}

func TestParseGeniusSearch(t *testing.T) {
	data, err := os.ReadFile("testdata/genius_search.json")
	if err != nil {
		t.Fatal(err)
	}
	songs, err := ParseGeniusSearch(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) == 0 || songs[0].Artist.Name != "Rick Astley" || !strings.HasPrefix(songs[0].URL, "https://genius.com/") {
		t.Fatalf("unexpected songs: %+v", songs)
	}
	official := []byte(`{"response":{"hits":[{"type":"song","result":{"full_title":"A by B","title":"A","url":"https://genius.com/a","primary_artist":{"name":"B"}}}]}}`)
	if songs, _ := ParseGeniusSearch(official); len(songs) != 1 || songs[0].Title != "A" {
		t.Fatalf("official API format not parsed: %+v", songs)
	}
}

func TestExtractLyrics(t *testing.T) {
	data, err := os.ReadFile("testdata/genius_song.html")
	if err != nil {
		t.Fatal(err)
	}
	lyrics, err := ExtractLyrics(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Never gonna give you up", "[Chorus]"} {
		if !strings.Contains(lyrics, want) {
			t.Fatalf("lyrics missing %q:\n%s", want, lyrics)
		}
	}
	for _, junk := range []string{"Contributors", "Translations"} {
		if strings.Contains(lyrics, junk) {
			t.Fatalf("lyrics contain header junk %q:\n%s", junk, lyrics[:300])
		}
	}
	t.Logf("%s", lyrics[:400])
	for _, c := range chunkLines(lyrics, 200) {
		if len([]rune(c)) > 200 {
			t.Fatalf("chunk too long: %d", len([]rune(c)))
		}
	}
}

func TestParseNixosBundle(t *testing.T) {
	js, err := os.ReadFile("testdata/nixos_frontend.js")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseNixosBundle(js)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != 51 || cfg.Username == "" || cfg.Password == "" || cfg.Channels["unstable"] != "nixos-unstable" || cfg.Channels[cfg.DefaultChannel] == "" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := ParseNixosBundle([]byte("nothing here")); err == nil {
		t.Fatal("expected error for unrecognised bundle")
	}
	html, _ := os.ReadFile("testdata/nixos_frontend.html")
	if m := bundleRe.FindSubmatch(html); m == nil || !strings.HasPrefix(string(m[1]), "/static/js/index.") {
		t.Fatalf("bundle path not found in %s", html)
	}
}

func TestParseNixosSearch(t *testing.T) {
	data, err := os.ReadFile("testdata/nixos_search.json")
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := ParseNixosSearch(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 || pkgs[0].AttrName != "ripgrep" || pkgs[0].Version == "" {
		t.Fatalf("unexpected packages: %+v", pkgs)
	}
	e := nixPackageEmbed(pkgs[0], "26.05", "nixos-26.05", "ripgrep")
	if !strings.Contains(e.Description, "nix shell nixpkgs#ripgrep") || !strings.Contains(e.URL, "show=ripgrep") {
		t.Fatalf("bad embed: %+v", e)
	}
	var src string
	for _, f := range e.Fields {
		if f.Name == "Source" {
			src = f.Value
		}
	}
	if !strings.Contains(src, "github.com/NixOS/nixpkgs/blob/nixos-26.05/pkgs/") || !strings.Contains(src, "#L") {
		t.Fatalf("bad source link: %q", src)
	}
	if _, err := ParseNixosSearch([]byte(`{"error":{"type":"index_not_found_exception"}}`)); err == nil {
		t.Fatal("expected backend error to surface")
	}
}

func TestNixosQuery(t *testing.T) {
	for _, docType := range []string{"package", "option"} {
		body, err := nixosQuery(docType, `fire"fox`, 10)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatalf("query is not valid JSON: %v", err)
		}
		if !strings.Contains(string(body), `"type":"`+docType+`"`) {
			t.Fatalf("%s query does not filter by type: %s", docType, body)
		}
	}
}

func TestPlatformSummary(t *testing.T) {
	if got := platformSummary([]string{"x86_64-linux", "aarch64-darwin", "aarch64-linux"}); got != "Linux, macOS" {
		t.Fatalf("got %q", got)
	}
}

func TestParseNixosOptions(t *testing.T) {
	data, err := os.ReadFile("testdata/nixos_options.json")
	if err != nil {
		t.Fatal(err)
	}
	opts, err := ParseNixosOptions(data)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]NixOption{}
	for _, o := range opts {
		byName[o.Name] = o
	}
	o, ok := byName["networking.firewall.allowedTCPPorts"]
	if !ok || o.Type == "" || o.Default == nil || o.Example == nil {
		t.Fatalf("firewall option not parsed: %+v", o)
	}
	e := nixOptionEmbed(o, "26.05", "nixos-26.05", "firewall")
	var fields []string
	for _, f := range e.Fields {
		fields = append(fields, f.Name+"="+f.Value)
	}
	all := strings.Join(fields, "\n")
	for _, want := range []string{"Default=```nix\n[ ]\n```", "Example=```nix\n[\n  22\n  80\n]\n```", "blob/nixos-26.05/nixos/modules/services/networking/firewall.nix"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in fields:\n%s", want, all)
		}
	}
	for _, o := range opts {
		e := nixOptionEmbed(o, "26.05", "nixos-26.05", "x")
		if strings.Contains(e.Description, "<") && strings.Contains(e.Description, "</") {
			t.Errorf("%s: HTML left in description: %s", o.Name, e.Description)
		}
		if len([]rune(e.Description)) > 4096 {
			t.Errorf("%s: description too long", o.Name)
		}
		for _, f := range e.Fields {
			if len([]rune(f.Value)) > 1024 {
				t.Errorf("%s: field %s too long", o.Name, f.Name)
			}
		}
	}
	t.Logf("%s", nixOptionEmbed(byName["users.users.<name>.hashedPassword"], "26.05", "nixos-26.05", "x").Description)
}

func TestWikiFeatured(t *testing.T) {
	data, err := os.ReadFile("testdata/wiki_featured.json")
	if err != nil {
		t.Fatal(err)
	}
	feed, err := ParseWikiFeatured(data)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	pages := featuredPages(feed, day)
	if len(pages) != 3 {
		t.Fatalf("expected featured, picture and most read pages, got %d", len(pages))
	}
	tfa, potd, most := pages[0].Embed, pages[1].Embed, pages[2].Embed
	if tfa.Title != "Valley Parade" || !strings.Contains(tfa.Author.Name, "27 September 2026") || !strings.HasPrefix(tfa.URL, "https://en.wikipedia.org/wiki/") {
		t.Fatalf("featured article: %+v", tfa)
	}
	if potd.Image == nil || strings.HasPrefix(potd.Title, "File:") || !strings.Contains(potd.Description, "Timothy A. Gonsalves") {
		t.Fatalf("picture of the day: %+v", potd)
	}
	if !strings.Contains(most.Description, "1. [Lizzie Borden](") || !strings.Contains(most.Description, "421,359 views") || !strings.Contains(most.Author.Name, "26 September 2026") {
		t.Fatalf("most read: %s", most.Description)
	}
	for _, p := range pages {
		if n := len([]rune(p.Embed.Description)); n > 4096 {
			t.Fatalf("description too long: %d", n)
		}
	}
}

func TestWikiRandomEmbed(t *testing.T) {
	data, err := os.ReadFile("testdata/wiki_random.json")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := ParseWikiSummary(data)
	if err != nil || sum.Type != "standard" {
		t.Fatal(err, sum)
	}
	e := wikiSummaryEmbed(*sum, "🎲 Random article")
	if e.Title != sum.Name() || e.URL == "" || !strings.Contains(e.Description, "[Read the article](") {
		t.Fatalf("bad embed: %+v", e)
	}
	if strings.Contains(e.Title, "_") {
		t.Fatalf("title not normalized: %q", e.Title)
	}
}

func TestParseDay(t *testing.T) {
	now := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{"": "2026-09-27", "today": "2026-09-27", "Yesterday": "2026-09-26", "2024-05-17": "2024-05-17"} {
		got, err := parseDay(in, now)
		if err != nil || got.Format("2006-01-02") != want {
			t.Errorf("parseDay(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"2026-09-28", "2010-01-01", "17/05/2024", "soon"} {
		if _, err := parseDay(bad, now); err == nil {
			t.Errorf("parseDay(%q) should fail", bad)
		}
	}
}

func TestFormatCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 421359: "421,359", 1234567: "1,234,567"} {
		if got := formatCount(n); got != want {
			t.Errorf("formatCount(%d) = %q", n, got)
		}
	}
}

func TestParseFrogTips(t *testing.T) {
	data, err := os.ReadFile("testdata/frogtips.json")
	if err != nil {
		t.Fatal(err)
	}
	tips, err := ParseFrogTips(data)
	if err != nil || len(tips) != 5 {
		t.Fatalf("got %d tips, %v", len(tips), err)
	}
	for _, tip := range tips {
		if tip == "" || tip != strings.ToUpper(tip) {
			t.Errorf("unexpected tip %q", tip)
		}
	}
}

func TestFrogTipsQueue(t *testing.T) {
	f := &frogTips{tips: []string{"ONE", "TWO"}}
	for _, want := range []string{"ONE", "TWO"} {
		if got, err := f.next(context.Background()); err != nil || got != want {
			t.Fatalf("next() = %q, %v; want %q", got, err, want)
		}
	}
	if len(f.tips) != 0 {
		t.Fatal("queue not drained")
	}
}
