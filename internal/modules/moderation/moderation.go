// Package moderation provides kick, ban, hackban, timeout and purge.
package moderation

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"mayuri2000/internal/cmd"
)

const category = "Moderation"

const maxTimeout = 28 * 24 * time.Hour

func reasonOpt() cmd.Option {
	return cmd.Option{Name: "reason", Description: "Reason (shown in the audit log)", Type: cmd.String, Rest: true, MaxLength: cmd.Ptr(400)}
}

func deleteDaysOpt() cmd.Option {
	return cmd.Option{
		Name: "delete_days", Description: "Delete this many days of their messages (0-7)", Type: cmd.Int, Min: cmd.Ptr(0), Max: cmd.Ptr(7),
		// A single digit, so user IDs are never mistaken for it.
		Accept: func(tok string) bool { return len(tok) == 1 && tok[0] >= '0' && tok[0] <= '7' },
	}
}

func Commands() []*cmd.Command {
	return []*cmd.Command{
		{
			Name: "kick", Category: category, Description: "Kick a member from the server",
			Perms: discord.PermissionKickMembers, GuildOnly: true,
			Options: []cmd.Option{
				{Name: "user", Description: "Member to kick", Type: cmd.User, Required: true},
				reasonOpt(),
			},
			Run: kick,
		},
		{
			Name: "ban", Category: category, Description: "Ban a user, whether or not they are in the server",
			Perms: discord.PermissionBanMembers, GuildOnly: true,
			Options: []cmd.Option{
				{Name: "user", Description: "User to ban", Type: cmd.User, Required: true},
				deleteDaysOpt(),
				reasonOpt(),
			},
			Run: ban,
		},
		{
			Name: "hackban", Aliases: []string{"massban"}, Category: category,
			Description: "Ban users by ID, even if they are not in the server",
			Perms:       discord.PermissionBanMembers, GuildOnly: true,
			Options: []cmd.Option{
				// For prefix use: `hackban <ids...> [delete_days] [reason...]`.
				{Name: "user_ids", Description: "User IDs separated by spaces or commas", Type: cmd.String, Required: true, Rest: true},
				deleteDaysOpt(),
				reasonOpt(),
			},
			Run: hackban,
		},
		{
			Name: "unban", Category: category, Description: "Unban a user by ID",
			Perms: discord.PermissionBanMembers, GuildOnly: true,
			Options: []cmd.Option{
				{Name: "user_id", Description: "ID of the user to unban", Type: cmd.String, Required: true},
				reasonOpt(),
			},
			Run: unban,
		},
		{
			Name: "timeout", Aliases: []string{"mute"}, Category: category,
			Description: "Time out a member (e.g. 10m, 1h30m, 2d; max 28d)",
			Perms:       discord.PermissionModerateMembers, GuildOnly: true,
			Options: []cmd.Option{
				{Name: "user", Description: "Member to time out", Type: cmd.User, Required: true},
				{Name: "duration", Description: "How long, e.g. 10m, 1h30m, 2d (max 28d)", Type: cmd.String, Required: true},
				reasonOpt(),
			},
			Run: timeout,
		},
		{
			Name: "untimeout", Aliases: []string{"unmute"}, Category: category,
			Description: "Remove a member's timeout",
			Perms:       discord.PermissionModerateMembers, GuildOnly: true,
			Options: []cmd.Option{
				{Name: "user", Description: "Member to release", Type: cmd.User, Required: true},
				reasonOpt(),
			},
			Run: untimeout,
		},
		{
			Name: "purge", Aliases: []string{"clean", "cleanup"}, Category: category,
			Description: "Bulk delete recent messages (up to 100, newer than 14 days)",
			Perms:       discord.PermissionManageMessages, GuildOnly: true,
			Options: []cmd.Option{
				{Name: "count", Description: "How many messages to delete", Type: cmd.Int, Required: true, Min: cmd.Ptr(1), Max: cmd.Ptr(100)},
				{Name: "user", Description: "Only delete messages from this user", Type: cmd.User},
			},
			Run: purge,
		},
	}
}

func auditReason(c *cmd.Context) rest.RequestOpt {
	reason := c.StringOr("reason", "No reason given")
	return rest.WithReason(fmt.Sprintf("%s (by %s, %s)", reason, c.Author.Username, c.Author.ID))
}

// topRole returns the highest role position of a member.
func topRole(c *cmd.Context, m *discord.Member) int {
	top := 0
	for _, r := range c.Client.Caches.MemberRoles(*m) {
		top = max(top, r.Position)
	}
	return top
}

func selfMember(c *cmd.Context) (*discord.Member, error) {
	if m, ok := c.Client.Caches.SelfMember(*c.GuildID); ok {
		return &m, nil
	}
	return c.FetchMember(c.Client.ID())
}

// checkBot verifies the bot itself holds perm.
func checkBot(c *cmd.Context, perm discord.Permissions) (*discord.Member, error) {
	self, err := selfMember(c)
	if err != nil {
		return nil, err
	}
	if !c.Client.Caches.MemberPermissions(*self).Has(perm) {
		return nil, cmd.Errorf("I need the %s permission to do that.", perm)
	}
	return self, nil
}

// checkHierarchy makes sure both the invoker and the bot outrank target.
func checkHierarchy(c *cmd.Context, target *discord.Member, action string) error {
	guild, ok := c.Client.Caches.Guild(*c.GuildID)
	if !ok {
		return errors.New("guild not cached")
	}
	switch target.User.ID {
	case c.Author.ID:
		return cmd.Errorf("You can't %s yourself.", action)
	case c.Client.ID():
		return cmd.Errorf("I'm not going to %s myself.", action)
	case guild.OwnerID:
		return cmd.Errorf("You can't %s the server owner.", action)
	}
	targetTop := topRole(c, target)
	if c.Author.ID != guild.OwnerID && c.Member != nil && topRole(c, c.Member) <= targetTop {
		return cmd.Errorf("You can't %s someone with an equal or higher role than you.", action)
	}
	self, err := selfMember(c)
	if err != nil {
		return err
	}
	if topRole(c, self) <= targetTop {
		return cmd.Errorf("I can't %s them: their highest role is above or equal to mine.", action)
	}
	return nil
}

// memberOrNil returns the guild member for id, or nil if they aren't in the
// server.
func memberOrNil(c *cmd.Context, id snowflake.ID) (*discord.Member, error) {
	m, err := c.FetchMember(id)
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMember, rest.JSONErrorCodeUnknownUser) {
		return nil, nil
	}
	return m, err
}

func notify(c *cmd.Context, userID snowflake.ID, action string) {
	guildName := "a server"
	if g, ok := c.Client.Caches.Guild(*c.GuildID); ok {
		guildName = g.Name
	}
	dm, err := c.Client.Rest.CreateDMChannel(userID)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("You were %s from **%s**.", action, guildName)
	if r, ok := c.OptString("reason"); ok {
		msg += "\nReason: " + r
	}
	_, _ = c.Client.Rest.CreateMessage(dm.ID(), discord.MessageCreate{Content: msg})
}

func kick(c *cmd.Context) error {
	u, _ := c.OptUser("user")
	if _, err := checkBot(c, discord.PermissionKickMembers); err != nil {
		return err
	}
	target, err := memberOrNil(c, u.ID)
	if err != nil {
		return err
	}
	if target == nil {
		return cmd.Errorf("%s isn't in this server.", u.Username)
	}
	if err := checkHierarchy(c, target, "kick"); err != nil {
		return err
	}
	notify(c, u.ID, "kicked")
	if err := c.Client.Rest.RemoveMember(*c.GuildID, u.ID, auditReason(c)); err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("👢 Kicked **%s** (`%s`).", u.Username, u.ID))
}

func deleteDuration(c *cmd.Context) time.Duration {
	return time.Duration(c.IntOr("delete_days", 0)) * 24 * time.Hour
}

func ban(c *cmd.Context) error {
	u, _ := c.OptUser("user")
	if _, err := checkBot(c, discord.PermissionBanMembers); err != nil {
		return err
	}
	target, err := memberOrNil(c, u.ID)
	if err != nil {
		return err
	}
	if target != nil {
		if err := checkHierarchy(c, target, "ban"); err != nil {
			return err
		}
		notify(c, u.ID, "banned")
	} else if u.ID == c.Author.ID || u.ID == c.Client.ID() {
		return cmd.Errorf("Nice try.")
	}
	if err := c.Client.Rest.AddBan(*c.GuildID, u.ID, deleteDuration(c), auditReason(c)); err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("🔨 Banned **%s** (`%s`).", u.Username, u.ID))
}

// ParseIDs extracts user IDs (raw or mentions) separated by spaces/commas.
func ParseIDs(s string) (ids []snowflake.ID, invalid []string) {
	seen := map[snowflake.ID]bool{}
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		id, ok := cmd.ParseUserID(tok)
		if !ok {
			invalid = append(invalid, tok)
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, invalid
}

const maxHackban = 50

func hackban(c *cmd.Context) error {
	if _, err := checkBot(c, discord.PermissionBanMembers); err != nil {
		return err
	}
	raw := c.String("user_ids")
	deleteDays := c.IntOr("delete_days", 0)
	reason := c.StringOr("reason", "No reason given")
	if c.SourceMessage != nil {
		raw, deleteDays, reason = splitHackbanArgs(raw)
	}
	ids, invalid := ParseIDs(raw)
	if len(ids) == 0 {
		return cmd.Errorf("Give me at least one valid user ID.")
	}
	if len(ids) > maxHackban {
		return cmd.Errorf("That's %d users; I'll only hackban up to %d at a time.", len(ids), maxHackban)
	}
	if err := c.Defer(); err != nil {
		return err
	}

	var lines []string
	for _, tok := range invalid {
		lines = append(lines, fmt.Sprintf("❔ `%s`: not a valid user ID", tok))
	}
	banned := 0
	opt := rest.WithReason(fmt.Sprintf("%s (by %s, %s)", reason, c.Author.Username, c.Author.ID))
	deleteFor := time.Duration(deleteDays) * 24 * time.Hour
	for _, id := range ids {
		lines = append(lines, hackbanOne(c, id, deleteFor, opt, &banned))
	}
	summary := fmt.Sprintf("Banned %d/%d user(s).\n", banned, len(ids))
	out := summary + strings.Join(lines, "\n")
	if len(out) > 1900 {
		out = out[:1900] + "\n…"
	}
	return c.Reply(out)
}

// splitHackbanArgs splits prefix input into the leading run of IDs, an
// optional single-digit delete_days and the remaining reason.
func splitHackbanArgs(s string) (ids string, deleteDays int, reason string) {
	fields := strings.Fields(s)
	i := 0
	for i < len(fields) {
		tok := strings.Trim(fields[i], ",")
		if _, ok := cmd.ParseUserID(tok); !ok && tok != "" {
			break
		}
		i++
	}
	ids = strings.Join(fields[:i], " ")
	if i < len(fields) && len(fields[i]) == 1 && fields[i][0] >= '0' && fields[i][0] <= '7' {
		deleteDays = int(fields[i][0] - '0')
		i++
	}
	reason = "No reason given"
	if i < len(fields) {
		reason = strings.Join(fields[i:], " ")
	}
	return ids, deleteDays, reason
}

func hackbanOne(c *cmd.Context, id snowflake.ID, deleteFor time.Duration, reason rest.RequestOpt, banned *int) string {
	if id == c.Author.ID || id == c.Client.ID() {
		return fmt.Sprintf("⛔ `%s`: refusing to ban that user", id)
	}
	user, err := c.Client.Rest.GetUser(id)
	if err != nil {
		if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownUser) {
			return fmt.Sprintf("❔ `%s`: no such user", id)
		}
		return fmt.Sprintf("⚠️ `%s`: %s", id, err)
	}
	if _, err := c.Client.Rest.GetBan(*c.GuildID, id); err == nil {
		return fmt.Sprintf("➖ **%s** (`%s`): already banned", user.Username, id)
	}
	member, err := memberOrNil(c, id)
	if err != nil {
		return fmt.Sprintf("⚠️ `%s`: %s", id, err)
	}
	if member != nil {
		if err := checkHierarchy(c, member, "ban"); err != nil {
			return fmt.Sprintf("⛔ **%s** (`%s`): %s", user.Username, id, err)
		}
	}
	if err := c.Client.Rest.AddBan(*c.GuildID, id, deleteFor, reason); err != nil {
		return fmt.Sprintf("⚠️ **%s** (`%s`): %s", user.Username, id, err)
	}
	*banned++
	return fmt.Sprintf("🔨 **%s** (`%s`): banned", user.Username, id)
}

func unban(c *cmd.Context) error {
	id, ok := cmd.ParseUserID(c.String("user_id"))
	if !ok {
		return cmd.Errorf("That isn't a valid user ID.")
	}
	if _, err := checkBot(c, discord.PermissionBanMembers); err != nil {
		return err
	}
	err := c.Client.Rest.DeleteBan(*c.GuildID, id, auditReason(c))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownBan) {
		return cmd.Errorf("`%s` isn't banned.", id)
	}
	if err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("Unbanned `%s`.", id))
}

// ParseDuration parses durations like "10m", "1h30m", "2d", "1w" or a bare
// number of minutes.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if n, err := parseUint(s); err == nil {
		return time.Duration(n) * time.Minute, nil
	}
	units := map[string]time.Duration{
		"s": time.Second, "sec": time.Second, "secs": time.Second, "second": time.Second, "seconds": time.Second,
		"m": time.Minute, "min": time.Minute, "mins": time.Minute, "minute": time.Minute, "minutes": time.Minute,
		"h": time.Hour, "hr": time.Hour, "hrs": time.Hour, "hour": time.Hour, "hours": time.Hour,
		"d": 24 * time.Hour, "day": 24 * time.Hour, "days": 24 * time.Hour,
		"w": 7 * 24 * time.Hour, "week": 7 * 24 * time.Hour, "weeks": 7 * 24 * time.Hour,
	}
	var total time.Duration
	for s != "" {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		n, err := parseUint(s[:i])
		if err != nil {
			return 0, err
		}
		s = s[i:]
		j := 0
		for j < len(s) && (s[j] < '0' || s[j] > '9') {
			j++
		}
		unit, ok := units[s[:j]]
		if !ok {
			return 0, fmt.Errorf("unknown unit %q", s[:j])
		}
		s = s[j:]
		total += time.Duration(n) * unit
		if total > 365*24*time.Hour {
			return 0, errors.New("duration too long")
		}
	}
	return total, nil
}

func parseUint(s string) (int64, error) {
	var n int64
	if s == "" || len(s) > 9 {
		return 0, errors.New("bad number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("bad number")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

func timeout(c *cmd.Context) error {
	u, _ := c.OptUser("user")
	d, err := ParseDuration(c.String("duration"))
	if err != nil {
		return cmd.Errorf("I don't understand that duration. Try something like `10m`, `1h30m` or `2d`.")
	}
	if d < time.Minute {
		return cmd.Errorf("Timeouts must be at least a minute.")
	}
	if d > maxTimeout {
		return cmd.Errorf("Discord only allows timeouts up to 28 days.")
	}
	if _, err := checkBot(c, discord.PermissionModerateMembers); err != nil {
		return err
	}
	target, err := memberOrNil(c, u.ID)
	if err != nil {
		return err
	}
	if target == nil {
		return cmd.Errorf("%s isn't in this server.", u.Username)
	}
	if err := checkHierarchy(c, target, "time out"); err != nil {
		return err
	}
	if c.Client.Caches.MemberPermissions(*target).Has(discord.PermissionAdministrator) {
		return cmd.Errorf("Administrators can't be timed out.")
	}
	until := time.Now().Add(d)
	_, err = c.Client.Rest.UpdateMember(*c.GuildID, u.ID, discord.MemberUpdate{
		CommunicationDisabledUntil: omit.NewPtr(until),
	}, auditReason(c))
	if err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("🔇 Timed out **%s** until <t:%d:f> (<t:%d:R>).", u.Username, until.Unix(), until.Unix()))
}

func untimeout(c *cmd.Context) error {
	u, _ := c.OptUser("user")
	if _, err := checkBot(c, discord.PermissionModerateMembers); err != nil {
		return err
	}
	target, err := memberOrNil(c, u.ID)
	if err != nil {
		return err
	}
	if target == nil {
		return cmd.Errorf("%s isn't in this server.", u.Username)
	}
	if err := checkHierarchy(c, target, "untime out"); err != nil {
		return err
	}
	_, err = c.Client.Rest.UpdateMember(*c.GuildID, u.ID, discord.MemberUpdate{
		CommunicationDisabledUntil: omit.NewNilPtr[time.Time](),
	}, auditReason(c))
	if err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("🔊 Removed the timeout for **%s**.", u.Username))
}

func purge(c *cmd.Context) error {
	if _, err := checkBot(c, discord.PermissionManageMessages); err != nil {
		return err
	}
	count := c.Int("count")
	filter, hasFilter := c.OptUser("user")

	var before snowflake.ID
	if c.SourceMessage != nil {
		before = c.SourceMessage.ID
		_ = c.Client.Rest.DeleteMessage(c.ChannelID, c.SourceMessage.ID)
	}
	msgs, err := c.Client.Rest.GetMessages(c.ChannelID, 0, before, 0, 100)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-14*24*time.Hour + time.Minute)
	var ids []snowflake.ID
	for _, m := range msgs {
		if len(ids) >= count {
			break
		}
		if m.ID.Time().Before(cutoff) {
			break
		}
		if hasFilter && m.Author.ID != filter.ID {
			continue
		}
		ids = append(ids, m.ID)
	}
	switch len(ids) {
	case 0:
		return cmd.Errorf("No messages to delete (only messages newer than 14 days can be purged).")
	case 1:
		err = c.Client.Rest.DeleteMessage(c.ChannelID, ids[0], rest.WithReason("purge by "+c.Author.Username))
	default:
		err = c.Client.Rest.BulkDeleteMessages(c.ChannelID, ids, rest.WithReason("purge by "+c.Author.Username))
	}
	if err != nil {
		return err
	}
	return c.ReplyEphemeral(fmt.Sprintf("🧹 Deleted %d message(s).", len(ids)))
}
