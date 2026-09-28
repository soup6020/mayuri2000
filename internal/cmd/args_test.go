package cmd

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	for in, want := range map[string][]string{
		`a b  c`:            {"a", "b", "c"},
		`"hello world" x`:   {"hello world", "x"},
		`'single quoted' y`: {"single quoted", "y"},
		`don't split`:       {"don't", "split"},
		`“smart quotes” z`:  {"smart quotes", "z"},
		"":                  nil,
	} {
		var got []string
		for _, tok := range tokenize(in) {
			got = append(got, tok.Value)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tokenize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseUserID(t *testing.T) {
	for in, ok := range map[string]bool{
		"<@123456789012345678>":  true,
		"<@!123456789012345678>": true,
		"123456789012345678":     true,
		"12345":                  false,
		"hello":                  false,
	} {
		if _, got := ParseUserID(in); got != ok {
			t.Errorf("ParseUserID(%q) = %v, want %v", in, got, ok)
		}
	}
}

func values(args []rawArg) map[string]string {
	out := map[string]string{}
	for _, a := range args {
		out[a.opt.Name] = a.value
	}
	return out
}

func TestAssign(t *testing.T) {
	isURL := func(s string) bool { return len(s) > 4 && s[:4] == "http" }
	imageOpts := []Option{
		{Name: "image", Type: Attachment},
		{Name: "url", Type: String, Accept: isURL},
		{Name: "user", Type: User, SlashOnly: true},
		{Name: "scale", Type: Int},
	}

	args, missing, extras := assign(imageOpts, "5")
	if got := values(args); !reflect.DeepEqual(got, map[string]string{"scale": "5"}) || missing != nil || extras != nil {
		t.Fatalf("optional url should be skipped for non-URL: %v %v %v", got, missing, extras)
	}

	args, _, extras = assign(imageOpts, "https://x/y.png 3 leftover")
	if got := values(args); got["url"] != "https://x/y.png" || got["scale"] != "3" || !reflect.DeepEqual(extras, []string{"leftover"}) {
		t.Fatalf("got %v extras %v", got, extras)
	}

	restOpts := []Option{
		{Name: "user", Type: User, Required: true},
		{Name: "days", Type: Int, Accept: func(s string) bool { return len(s) == 1 }},
		{Name: "reason", Type: String, Rest: true},
	}
	args, _, _ = assign(restOpts, `<@123456789012345678> being "rude" again`)
	if got := values(args); got["reason"] != `being "rude" again` || got["days"] != "" {
		t.Fatalf("rest parsing: %v", got)
	}
	args, _, _ = assign(restOpts, `123456789012345678 7 "quoted reason"`)
	if got := values(args); got["reason"] != "quoted reason" || got["days"] != "7" {
		t.Fatalf("quoted rest: %v", got)
	}

	_, missing, _ = assign(restOpts, "")
	if len(missing) != 1 || missing[0].Name != "user" {
		t.Fatalf("expected missing user, got %v", missing)
	}
}

func TestUsage(t *testing.T) {
	c := &Command{Name: "ban", Options: []Option{
		{Name: "user", Type: User, Required: true},
		{Name: "image", Type: Attachment},
		{Name: "reason", Type: String, Rest: true},
	}}
	if got := Usage("%", c); got != "%ban <user> [reason...]" {
		t.Fatalf("got %q", got)
	}
}

func TestSlashCreateOrdersRequiredFirst(t *testing.T) {
	c := &Command{Name: "x", Description: "d", Options: []Option{
		{Name: "opt", Type: String},
		{Name: "req", Type: String, Required: true},
	}}
	sc := c.slashCreate()
	if sc.Options[0].OptionName() != "req" {
		t.Fatalf("required option not first: %v", sc.Options[0].OptionName())
	}
}
