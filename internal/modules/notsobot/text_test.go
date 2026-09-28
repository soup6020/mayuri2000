package notsobot

import "testing"

func TestAesthetics(t *testing.T) {
	if got := Aesthetics("hi 1!"); got != "ｈｉ　１！" {
		t.Fatalf("got %q", got)
	}
}

func TestEmojify(t *testing.T) {
	if got := Emojify("Ab1"); got != "🇦​🇧​1️⃣" {
		t.Fatalf("got %q", got)
	}
}
