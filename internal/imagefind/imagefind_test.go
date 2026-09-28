package imagefind

import "testing"

func TestLooksLikeImageRef(t *testing.T) {
	for in, want := range map[string]bool{
		"https://cdn.discordapp.com/a.png": true,
		"<:blob:123456789012345678>":       true,
		"<a:blob:123456789012345678>":      true,
		"<@123456789012345678>":            true,
		"123456789012345678":               true,
		"90":                               false,
		"white":                            false,
	} {
		if got := LooksLikeImageRef(in); got != want {
			t.Errorf("LooksLikeImageRef(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestImageLinkRe(t *testing.T) {
	text := "look https://cdn.discordapp.com/attachments/1/2/cat.PNG?ex=abc&is=def and https://example.com/page"
	got := imageLinkRe.FindAllString(text, -1)
	if len(got) != 1 || got[0] != "https://cdn.discordapp.com/attachments/1/2/cat.PNG?ex=abc&is=def" {
		t.Fatalf("got %q", got)
	}
	if m := tenorRe.FindStringSubmatch("https://tenor.com/view/cat-dance-12345678"); m == nil || m[1] != "12345678" {
		t.Fatalf("tenor regex failed: %v", m)
	}
}
