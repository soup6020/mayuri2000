package core

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
)

func userInfoCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "userinfo",
		Aliases:     []string{"ui", "whois", "user"},
		Category:    "Info",
		Description: "Show information about a user",
		Options: []cmd.Option{
			{Name: "user", Description: "User to look up (default: you)", Type: cmd.User},
		},
		Run: userInfo,
	}
}

func roleInfoCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "roleinfo",
		Aliases:     []string{"ri", "role"},
		Category:    "Info",
		Description: "Show information about a role",
		GuildOnly:   true,
		Options: []cmd.Option{
			{Name: "role", Description: "Role mention, ID or name", Type: cmd.Role, Required: true, Rest: true},
		},
		Run: roleInfo,
	}
}

// UserInfoMenu is the "User Info" user context menu.
func UserInfoMenu() *cmd.Menu {
	return &cmd.Menu{Name: "User Info", Kind: cmd.MenuUser, Run: userInfo}
}

// keyPermissions lists the permissions worth calling out, in display order.
var keyPermissions = []struct {
	perm discord.Permissions
	name string
}{
	{discord.PermissionAdministrator, "Administrator"},
	{discord.PermissionManageGuild, "Manage Server"},
	{discord.PermissionManageRoles, "Manage Roles"},
	{discord.PermissionManageChannels, "Manage Channels"},
	{discord.PermissionBanMembers, "Ban Members"},
	{discord.PermissionKickMembers, "Kick Members"},
	{discord.PermissionModerateMembers, "Timeout Members"},
	{discord.PermissionManageMessages, "Manage Messages"},
	{discord.PermissionManageNicknames, "Manage Nicknames"},
	{discord.PermissionManageWebhooks, "Manage Webhooks"},
	{discord.PermissionManageGuildExpressions, "Manage Expressions"},
	{discord.PermissionManageEvents, "Manage Events"},
	{discord.PermissionManageThreads, "Manage Threads"},
	{discord.PermissionViewAuditLog, "View Audit Log"},
	{discord.PermissionMentionEveryone, "Mention Everyone"},
	{discord.PermissionMuteMembers, "Mute Members"},
	{discord.PermissionDeafenMembers, "Deafen Members"},
	{discord.PermissionMoveMembers, "Move Members"},
}

// KeyPermissions names the notable permissions in p.
func KeyPermissions(p discord.Permissions) string {
	if p.Has(discord.PermissionAdministrator) {
		return "Administrator (all permissions)"
	}
	var names []string
	for _, k := range keyPermissions {
		if p.Has(k.perm) {
			names = append(names, k.name)
		}
	}
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, ", ")
}

var badges = []struct {
	flag discord.UserFlags
	name string
}{
	{discord.UserFlagDiscordEmployee, "Discord Staff"},
	{discord.UserFlagPartneredServerOwner, "Partnered Server Owner"},
	{discord.UserFlagDiscordCertifiedModerator, "Moderator Programs Alumni"},
	{discord.UserFlagHypeSquadEvents, "HypeSquad Events"},
	{discord.UserFlagHouseBravery, "HypeSquad Bravery"},
	{discord.UserFlagHouseBrilliance, "HypeSquad Brilliance"},
	{discord.UserFlagHouseBalance, "HypeSquad Balance"},
	{discord.UserFlagBugHunterLevel1, "Bug Hunter"},
	{discord.UserFlagBugHunterLevel2, "Bug Hunter (Gold)"},
	{discord.UserFlagEarlySupporter, "Early Supporter"},
	{discord.UserFlagEarlyVerifiedBotDeveloper, "Early Verified Bot Developer"},
	{discord.UserFlagVerifiedBot, "Verified Bot"},
}

func timestamp(t time.Time) string {
	return fmt.Sprintf("<t:%d:D> (<t:%d:R>)", t.Unix(), t.Unix())
}

// joinLimited joins items with sep, stopping with "and N more" before the
// result would exceed limit characters.
func joinLimited(items []string, sep string, limit int) string {
	var b strings.Builder
	for i, it := range items {
		more := fmt.Sprintf(" and %d more", len(items)-i)
		if b.Len()+len(sep)+len(it)+len(more) > limit {
			b.WriteString(more)
			break
		}
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(it)
	}
	return b.String()
}

func inline(name, value string) discord.EmbedField {
	return discord.EmbedField{Name: name, Value: value, Inline: cmd.Ptr(true)}
}

func userInfo(c *cmd.Context) error {
	target := c.Author
	if c.TargetUser != nil {
		target = *c.TargetUser
	} else if u, ok := c.OptUser("user"); ok {
		target = u
	}
	// Fetch the full user: banners and accent colours aren't in cached users.
	if u, err := c.Client.Rest.GetUser(target.ID); err == nil {
		target = *u
	}

	var member *discord.Member
	if c.GuildID != nil {
		m, err := c.FetchMember(target.ID)
		switch {
		case err == nil:
			member = m
		case !rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMember, rest.JSONErrorCodeUnknownUser):
			return err
		}
	}

	title := target.Username
	if target.GlobalName != nil && *target.GlobalName != target.Username {
		title = fmt.Sprintf("%s (@%s)", *target.GlobalName, target.Username)
	}
	if target.Bot {
		title += " 🤖"
	}
	e := discord.Embed{
		Title:  title,
		Fields: []discord.EmbedField{inline("ID", target.ID.String()), inline("Created", timestamp(target.ID.Time()))},
		Footer: &discord.EmbedFooter{Text: "User " + target.ID.String()},
	}
	e.Thumbnail = &discord.EmbedResource{URL: target.EffectiveAvatarURL(discord.WithSize(256))}
	if target.AccentColor != nil {
		e.Color = *target.AccentColor
	}
	if banner := target.BannerURL(discord.WithSize(1024)); banner != nil {
		e.Image = &discord.EmbedResource{URL: *banner}
	}

	var badgeNames []string
	for _, b := range badges {
		if target.PublicFlags.Has(b.flag) {
			badgeNames = append(badgeNames, b.name)
		}
	}
	if len(badgeNames) > 0 {
		e.Fields = append(e.Fields, inline("Badges", strings.Join(badgeNames, ", ")))
	}

	if member == nil {
		if c.GuildID != nil {
			e.Description = "Not a member of this server."
		}
		return c.ReplyEmbeds(e)
	}

	if member.Avatar != nil {
		e.Thumbnail = &discord.EmbedResource{URL: member.EffectiveAvatarURL(discord.WithSize(256))}
	}
	if banner := member.BannerURL(discord.WithSize(1024)); banner != nil {
		e.Image = &discord.EmbedResource{URL: *banner}
	}
	if member.JoinedAt != nil {
		e.Fields = append(e.Fields, inline("Joined", timestamp(*member.JoinedAt)))
	}
	if member.Nick != nil {
		e.Fields = append(e.Fields, inline("Nickname", *member.Nick))
	}
	if member.PremiumSince != nil {
		e.Fields = append(e.Fields, inline("Boosting since", timestamp(*member.PremiumSince)))
	}
	if until := member.CommunicationDisabledUntil; until != nil && until.After(time.Now()) {
		e.Fields = append(e.Fields, inline("Timed out until", timestamp(*until)))
	}
	if g, ok := c.Client.Caches.Guild(*c.GuildID); ok && g.OwnerID == target.ID {
		e.Description = "👑 Server owner"
	}

	roles := c.Client.Caches.MemberRoles(*member)
	slices.SortFunc(roles, func(a, b discord.Role) int { return b.Position - a.Position })
	for _, r := range roles {
		if r.Color != 0 {
			e.Color = r.Color
			break
		}
	}
	mentions := make([]string, len(roles))
	for i, r := range roles {
		mentions[i] = r.Mention()
	}
	roleText := "None"
	if len(mentions) > 0 {
		roleText = joinLimited(mentions, " ", 1024)
	}
	e.Fields = append(e.Fields,
		discord.EmbedField{Name: fmt.Sprintf("Roles (%d)", len(roles)), Value: roleText},
		discord.EmbedField{Name: "Key permissions", Value: KeyPermissions(c.Client.Caches.MemberPermissions(*member))},
	)
	return c.ReplyEmbeds(e)
}

func roleInfo(c *cmd.Context) error {
	role, _ := c.OptRole("role")
	// Prefer the cached copy, which is complete even when the slash
	// command's resolved data is partial.
	if cached, ok := c.Client.Caches.Role(*c.GuildID, role.ID); ok {
		role = cached
	}

	total := 0
	for range c.Client.Caches.Roles(*c.GuildID) {
		total++
	}
	members := "Unknown"
	if counts, err := c.Client.Rest.GetRoleMemberCounts(*c.GuildID); err == nil {
		members = fmt.Sprint(counts[role.ID])
	}
	if role.ID == *c.GuildID { // @everyone
		if g, ok := c.Client.Caches.Guild(*c.GuildID); ok {
			members = fmt.Sprint(g.MemberCount)
		}
	}

	title := role.Name
	if role.Emoji != nil {
		title = *role.Emoji + " " + title
	}
	color := "Default"
	if role.Color != 0 {
		color = fmt.Sprintf("#%06X", role.Color)
	}
	e := discord.Embed{
		Title: title,
		Color: role.Color,
		Fields: []discord.EmbedField{
			inline("Mention", role.Mention()),
			inline("Members", members),
			inline("Created", timestamp(role.ID.Time())),
			inline("Colour", color),
			inline("Position", fmt.Sprintf("%d of %d", role.Position, total-1)),
			inline("Hoisted", yesNo(role.Hoist)),
			inline("Mentionable", yesNo(role.Mentionable)),
		},
		Footer: &discord.EmbedFooter{Text: "Role " + role.ID.String()},
	}
	if managed := managedBy(role); managed != "" {
		e.Fields = append(e.Fields, inline("Managed by", managed))
	}
	e.Fields = append(e.Fields, discord.EmbedField{Name: "Key permissions", Value: KeyPermissions(role.Permissions)})
	if icon := role.IconURL(discord.WithSize(128)); icon != nil {
		e.Thumbnail = &discord.EmbedResource{URL: *icon}
	}
	return c.ReplyEmbeds(e)
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func managedBy(r discord.Role) string {
	if !r.Managed && r.Tags == nil {
		return ""
	}
	switch {
	case r.Tags != nil && r.Tags.BotID != nil:
		return fmt.Sprintf("Bot <@%s>", *r.Tags.BotID)
	case r.Tags != nil && r.Tags.PremiumSubscriber:
		return "Server Boosting"
	case r.Tags != nil && r.Tags.SubscriptionListingID != nil:
		return "Server Subscription"
	case r.Tags != nil && r.Tags.GuildConnections:
		return "Linked Roles"
	case r.Managed:
		return "Integration"
	}
	return ""
}

const (
	memberPageSize = 1000
	// maxMemberScan bounds /inrole on very large servers (25 requests).
	maxMemberScan = 25000
)

// memberLister is the part of the REST client /inrole needs.
type memberLister interface {
	GetMembers(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]discord.Member, error)
}

// errMembersIntent explains the one setup step listing members depends on.
var errMembersIntent = cmd.Errorf("I can't list server members because the **Server Members** intent is disabled. " +
	"The bot owner needs to enable it in the Discord Developer Portal (Bot → Privileged Gateway Intents).")

// roleMembers pages through the server's members and returns those holding
// roleID (everyone, for the @everyone role). truncated reports that the scan
// stopped at maxMemberScan with more members possibly remaining.
func roleMembers(l memberLister, guildID, roleID snowflake.ID) (members []discord.Member, truncated bool, err error) {
	everyone := roleID == guildID
	var after snowflake.ID
	for scanned := 0; scanned < maxMemberScan; {
		page, err := l.GetMembers(guildID, memberPageSize, after)
		if err != nil {
			// Listing members needs the Server Members privileged intent;
			// without it Discord answers with Missing Access.
			if rest.IsJSONErrorCode(err, rest.JSONErrorCodeMissingAccess) {
				return nil, false, errMembersIntent
			}
			return nil, false, err
		}
		for _, m := range page {
			if everyone || slices.Contains(m.RoleIDs, roleID) {
				members = append(members, m)
			}
		}
		scanned += len(page)
		if len(page) < memberPageSize {
			return members, false, nil
		}
		after = page[len(page)-1].User.ID
	}
	return members, true, nil
}

const membersPerPage = 20

func inRoleCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "inrole",
		Aliases:     []string{"rolemembers", "whohas", "members"},
		Category:    "Info",
		Description: "List the members who have a role",
		GuildOnly:   true,
		// Paging the member list can exceed Discord's 3s reply window.
		Heavy: true,
		Options: []cmd.Option{
			{Name: "role", Description: "Role mention, ID or name", Type: cmd.Role, Required: true, Rest: true},
		},
		Run: inRole,
	}
}

// memberDisplayName is what Discord shows: server nickname, then global
// name, then username.
func memberDisplayName(m discord.Member) string {
	if m.Nick != nil && *m.Nick != "" {
		return *m.Nick
	}
	return m.User.EffectiveName()
}

// roleMemberPages renders members sorted by display name, 20 per page.
func roleMemberPages(role discord.Role, members []discord.Member, truncated bool) []cmd.Page {
	sorted := slices.Clone(members)
	slices.SortStableFunc(sorted, func(a, b discord.Member) int {
		if c := strings.Compare(strings.ToLower(memberDisplayName(a)), strings.ToLower(memberDisplayName(b))); c != 0 {
			return c
		}
		return strings.Compare(a.User.Username, b.User.Username)
	})
	footer := fmt.Sprintf("%d member(s)", len(sorted))
	if truncated {
		footer += fmt.Sprintf(" · may be incomplete: only the first %d server members were scanned", maxMemberScan)
	}
	var pages []cmd.Page
	for i := 0; i < len(sorted); i += membersPerPage {
		var b strings.Builder
		for _, m := range sorted[i:min(i+membersPerPage, len(sorted))] {
			fmt.Fprintf(&b, "%s `%s`\n", m.User.Mention(), m.User.Username)
		}
		pages = append(pages, cmd.Page{Embed: &discord.Embed{
			Title:       truncateRunes(fmt.Sprintf("Members with %s (%d)", role.Name, len(sorted)), 256),
			Description: b.String(),
			Color:       role.Color,
			Footer:      &discord.EmbedFooter{Text: footer},
		}})
	}
	return pages
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func inRole(c *cmd.Context) error {
	role, _ := c.OptRole("role")
	members, truncated, err := roleMembers(c.Client.Rest, *c.GuildID, role.ID)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		// Mentions in replies don't ping (allowed mentions are disabled).
		return c.Reply(fmt.Sprintf("%s has no members.", role.Mention()))
	}
	return c.Paginate(roleMemberPages(role, members, truncated))
}
