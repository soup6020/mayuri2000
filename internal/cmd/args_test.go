package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
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

func TestSlashAutocompleteFlag(t *testing.T) {
	c := &Command{Name: "x", Description: "d", Options: []Option{
		{Name: "zone", Description: "z", Type: String, Autocomplete: func(string) []Choice { return nil }},
	}}
	opt := c.slashCreate().Options[0].(discord.ApplicationCommandOptionString)
	if !opt.Autocomplete {
		t.Fatal("autocomplete flag not set on slash option")
	}
}

func TestFindRole(t *testing.T) {
	roles := []discord.Role{
		{ID: 111111111111111111, Name: "@everyone"},
		{ID: 222222222222222222, Name: "Moderators"},
		{ID: 333333333333333333, Name: "Mod Squad"},
		{ID: 444444444444444444, Name: "Server Booster"},
	}
	for query, want := range map[string]snowflake.ID{
		"<@&222222222222222222>": 222222222222222222,
		"333333333333333333":     333333333333333333,
		"moderators":             222222222222222222,
		"@Moderators":            222222222222222222,
		"booster":                444444444444444444,
		"everyone":               111111111111111111,
		"mod squad":              333333333333333333,
	} {
		r, err := FindRole(roles, query)
		if err != nil || r.ID != want {
			t.Errorf("FindRole(%q) = %v, %v; want %v", query, r.ID, err, want)
		}
	}
	if _, err := FindRole(roles, "mod"); err == nil || !strings.Contains(err.Error(), "several") {
		t.Errorf("ambiguous prefix should error, got %v", err)
	}
	if _, err := FindRole(roles, "nope"); err == nil {
		t.Error("unknown role should error")
	}
}

func TestAssignRoleRest(t *testing.T) {
	args, _, _ := assign([]Option{{Name: "role", Type: Role, Required: true, Rest: true}}, "Server Booster")
	if len(args) != 1 || args[0].value != "Server Booster" {
		t.Fatalf("got %+v", args)
	}
}

func TestFindChannel(t *testing.T) {
	var chans []discord.GuildChannel
	for id, name := range map[snowflake.ID]string{111111111111111111: "general", 222222222222222222: "news", 333333333333333333: "News"} {
		var ch discord.GuildTextChannel
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"id":"%d","type":0,"name":%q}`, id, name)), &ch); err != nil {
			t.Fatal(err)
		}
		chans = append(chans, ch)
	}
	for q, want := range map[string]snowflake.ID{
		"<#111111111111111111>": 111111111111111111,
		"111111111111111111":    111111111111111111,
		"#general":              111111111111111111,
		"GENERAL":               111111111111111111,
	} {
		if got, err := FindChannel(chans, q); err != nil || got != want {
			t.Errorf("FindChannel(%q) = %v, %v", q, got, err)
		}
	}
	for _, bad := range []string{"#news", "999999999999999999", "#nope"} {
		if _, err := FindChannel(chans, bad); err == nil {
			t.Errorf("FindChannel(%q) should fail", bad)
		}
	}
}

func TestSubcommands(t *testing.T) {
	group := &Command{Name: "rss", Description: "feeds", Subcommands: []*Command{
		{Name: "add", Description: "add a feed", Options: []Option{
			{Name: "name", Description: "n", Type: String, Required: true},
			{Name: "channel", Description: "c", Type: Channel},
		}, Run: func(*Context) error { return nil }},
		{Name: "remove", Aliases: []string{"del"}, Description: "remove", Run: func(*Context) error { return nil }},
	}}
	r := &Registry{byName: map[string]*Command{}}
	r.Add(group)

	sc := group.slashCreate()
	sub, ok := sc.Options[0].(discord.ApplicationCommandOptionSubCommand)
	if !ok || sub.Name != "add" || len(sub.Options) != 2 {
		t.Fatalf("bad subcommand create: %+v", sc.Options)
	}
	if _, ok := sub.Options[1].(discord.ApplicationCommandOptionChannel); !ok {
		t.Fatalf("channel option not created: %+v", sub.Options[1])
	}
	if s, ok := group.Sub("DEL"); !ok || s.FullName() != "rss remove" {
		t.Fatal("subcommand alias lookup failed")
	}
	if got := Usage("%", group.Subcommands[0]); got != "%rss add <name> [channel]" {
		t.Fatalf("usage %q", got)
	}
	if got := Usage("%", group); got != "%rss <add|remove> ..." {
		t.Fatalf("group usage %q", got)
	}
	if !strings.Contains(GroupUsage("%", group), "`%rss remove` remove") {
		t.Fatalf("group help: %s", GroupUsage("%", group))
	}
}
