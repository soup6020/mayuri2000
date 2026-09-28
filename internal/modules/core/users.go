package core

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"

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
