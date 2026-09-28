package notsobot

import "testing"

func TestAesthetics(t *testing.T) {
	if got := Aesthetics("hi 1!"); got != "ｈｉ　１！" {
		t.Fatalf("got %q", got)
	}
}

func TestEmojify(t *testing.T) {
	if got := Emojify("Ab1"); got != "\U0001F1E6\u200b\U0001F1E7\u200b1\uFE0F\u20E3" {
		t.Fatalf("got %q", got)
	}
}
