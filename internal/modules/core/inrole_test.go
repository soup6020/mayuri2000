package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const testGuild snowflake.ID = 1

// fakeLister serves members in ID order like Discord's list endpoint.
type fakeLister struct {
	members []discord.Member
	err     error
	calls   int
}

func (f *fakeLister) GetMembers(_ snowflake.ID, limit int, after snowflake.ID, _ ...rest.RequestOpt) ([]discord.Member, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var page []discord.Member
	for _, m := range f.members {
		if m.User.ID > after && len(page) < limit {
			page = append(page, m)
		}
	}
	return page, nil
}

// members makes n members with IDs 100.., every k-th holding role 50.
func members(n, k int) []discord.Member {
	out := make([]discord.Member, n)
	for i := range out {
		out[i] = discord.Member{User: discord.User{ID: snowflake.ID(100 + i), Username: "user"}}
		if i%k == 0 {
			out[i].RoleIDs = []snowflake.ID{50}
		}
	}
	return out
}

func TestRoleMembersPaging(t *testing.T) {
	f := &fakeLister{members: members(2500, 10)}
	got, truncated, err := roleMembers(f, testGuild, 50)
	if err != nil || truncated {
		t.Fatalf("err=%v truncated=%v", err, truncated)
	}
	if len(got) != 250 || f.calls != 3 {
		t.Fatalf("got %d members in %d calls, want 250 in 3", len(got), f.calls)
	}
	// @everyone (the guild ID) matches every member.
	all, _, _ := roleMembers(&fakeLister{members: members(30, 1000)}, testGuild, testGuild)
	if len(all) != 30 {
		t.Fatalf("@everyone: got %d members", len(all))
	}
}

func TestRoleMembersEmptyRole(t *testing.T) {
	got, truncated, err := roleMembers(&fakeLister{members: members(40, 1000)}, testGuild, 999)
	if err != nil || truncated || len(got) != 0 {
		t.Fatalf("got %d, truncated=%v, err=%v", len(got), truncated, err)
	}
}

func TestRoleMembersCap(t *testing.T) {
	f := &fakeLister{members: members(maxMemberScan+1500, 1)}
	got, truncated, err := roleMembers(f, testGuild, 50)
	if err != nil || !truncated || len(got) != maxMemberScan || f.calls != maxMemberScan/memberPageSize {
		t.Fatalf("got %d in %d calls, truncated=%v, err=%v", len(got), f.calls, truncated, err)
	}
}

func TestRoleMembersIntentDisabled(t *testing.T) {
	_, _, err := roleMembers(&fakeLister{err: &rest.Error{Code: rest.JSONErrorCodeMissingAccess}}, testGuild, 50)
	if !errors.Is(err, errMembersIntent) {
		t.Fatalf("Missing Access should map to the intent message, got %v", err)
	}
	other := errors.New("discord 500")
	if _, _, err := roleMembers(&fakeLister{err: other}, testGuild, 50); !errors.Is(err, other) {
		t.Fatalf("other errors must pass through unchanged, got %v", err)
	}
}

func named(id snowflake.ID, username string, global, nick string) discord.Member {
	m := discord.Member{User: discord.User{ID: id, Username: username}}
	if global != "" {
		m.User.GlobalName = &global
	}
	if nick != "" {
		m.Nick = &nick
	}
	return m
}

func TestRoleMemberPagesOrdering(t *testing.T) {
	role := discord.Role{ID: 50, Name: "Game Night", Color: 0xFF0000}
	ms := []discord.Member{
		named(1, "zed", "", ""),
		named(2, "amy", "Zoe", ""),       // global name wins over username
		named(3, "bob", "Bobby", "able"), // nickname wins over everything
		named(4, "carl", "", ""),
		named(5, "dan", "Ben", ""),
	}
	pages := roleMemberPages(role, ms, false)
	if len(pages) != 1 {
		t.Fatalf("%d pages", len(pages))
	}
	e := pages[0].Embed
	want := "<@3> `bob`\n<@5> `dan`\n<@4> `carl`\n<@1> `zed`\n<@2> `amy`\n"
	if e.Description != want {
		t.Fatalf("order:\n%s\nwant:\n%s", e.Description, want)
	}
	if e.Title != "Members with Game Night (5)" || e.Color != 0xFF0000 || e.Footer.Text != "5 member(s)" {
		t.Fatalf("embed: %+v %+v", e, e.Footer)
	}
}

func TestRoleMemberPagesPaging(t *testing.T) {
	ms := members(45, 1)
	pages := roleMemberPages(discord.Role{Name: "r"}, ms, false)
	if len(pages) != 3 {
		t.Fatalf("45 members → %d pages, want 3", len(pages))
	}
	for i, want := range []int{20, 20, 5} {
		d := pages[i].Embed.Description
		if n := strings.Count(d, "\n"); n != want {
			t.Errorf("page %d has %d lines, want %d", i, n, want)
		}
		if len([]rune(d)) > 4096 {
			t.Errorf("page %d description too long", i)
		}
	}
	// Worst case: 32 character usernames and 19 digit IDs still fit.
	long := make([]discord.Member, membersPerPage)
	for i := range long {
		long[i] = discord.Member{User: discord.User{ID: snowflake.ID(1e18) + snowflake.ID(i), Username: strings.Repeat("x", 32)}}
	}
	if d := roleMemberPages(discord.Role{Name: "r"}, long, false)[0].Embed.Description; len([]rune(d)) > 4096 {
		t.Fatalf("worst-case page is %d characters", len([]rune(d)))
	}
}

func TestRoleMemberPagesTruncated(t *testing.T) {
	pages := roleMemberPages(discord.Role{Name: "r"}, members(3, 1), true)
	if !strings.Contains(pages[0].Embed.Footer.Text, "may be incomplete") {
		t.Fatalf("footer: %q", pages[0].Embed.Footer.Text)
	}
}
