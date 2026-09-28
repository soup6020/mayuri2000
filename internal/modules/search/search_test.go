package search

import (
	"os"
	"strings"
	"testing"
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
