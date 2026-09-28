package core

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/discord"

	"mayuri2000/internal/cmd"
)

func serverInfoCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "serverinfo",
		Aliases:     []string{"guildinfo", "si"},
		Category:    "Info",
		Description: "Show information about this server",
		GuildOnly:   true,
		Run:         serverInfo,
	}
}

func statsCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "stats",
		Aliases:     []string{"botstats", "uptime"},
		Category:    "Info",
		Description: "Show bot uptime and usage statistics",
		Run:         stats,
	}
}

var notableFeatures = map[discord.GuildFeature]string{
	"COMMUNITY":              "Community",
	"PARTNERED":              "Partnered",
	"VERIFIED":               "Verified",
	"DISCOVERABLE":           "Discoverable",
	"VANITY_URL":             "Vanity URL",
	"ANIMATED_ICON":          "Animated icon",
	"BANNER":                 "Banner",
	"ROLE_ICONS":             "Role icons",
	"NEWS":                   "Announcement channels",
	"WELCOME_SCREEN_ENABLED": "Welcome screen",
}

func serverInfo(c *cmd.Context) error {
	g, err := c.Client.Rest.GetGuild(*c.GuildID, true)
	if err != nil {
		return err
	}

	var text, voice, categories, threads, other int
	for ch := range c.Client.Caches.ChannelsForGuild(g.ID) {
		switch ch.Type() {
		case discord.ChannelTypeGuildText, discord.ChannelTypeGuildNews, discord.ChannelTypeGuildForum, discord.ChannelTypeGuildMedia:
			text++
		case discord.ChannelTypeGuildVoice, discord.ChannelTypeGuildStageVoice:
			voice++
		case discord.ChannelTypeGuildCategory:
			categories++
		case discord.ChannelTypeGuildPublicThread, discord.ChannelTypeGuildPrivateThread, discord.ChannelTypeGuildNewsThread:
			threads++
		default:
			other++
		}
	}
	animated := 0
	for _, e := range g.Emojis {
		if e.Animated {
			animated++
		}
	}
	var features []string
	for _, f := range g.Features {
		if name, ok := notableFeatures[f]; ok {
			features = append(features, name)
		}
	}

	created := g.ID.Time().Unix()
	e := discord.Embed{
		Title: g.Name,
		Color: 0x5865F2,
		Fields: []discord.EmbedField{
			{Name: "Owner", Value: fmt.Sprintf("<@%s>", g.OwnerID), Inline: cmd.Ptr(true)},
			{Name: "Created", Value: fmt.Sprintf("<t:%d:D> (<t:%d:R>)", created, created), Inline: cmd.Ptr(true)},
			{Name: "Members", Value: fmt.Sprintf("%d (%d online)", g.ApproximateMemberCount, g.ApproximatePresenceCount), Inline: cmd.Ptr(true)},
			{Name: "Channels", Value: fmt.Sprintf("%d text · %d voice · %d categories", text, voice, categories), Inline: cmd.Ptr(true)},
			{Name: "Roles", Value: fmt.Sprint(len(g.Roles)), Inline: cmd.Ptr(true)},
			{Name: "Emoji / Stickers", Value: fmt.Sprintf("%d (%d animated) / %d", len(g.Emojis), animated, len(g.Stickers)), Inline: cmd.Ptr(true)},
			{Name: "Boosts", Value: fmt.Sprintf("Level %d (%d boosts)", g.PremiumTier, g.PremiumSubscriptionCount), Inline: cmd.Ptr(true)},
			{Name: "Verification", Value: verificationName(g.VerificationLevel), Inline: cmd.Ptr(true)},
			{Name: "Locale", Value: g.PreferredLocale, Inline: cmd.Ptr(true)},
		},
		Footer: &discord.EmbedFooter{Text: "ID: " + g.ID.String()},
	}
	if g.Description != nil && *g.Description != "" {
		e.Description = *g.Description
	}
	if g.VanityURLCode != nil {
		e.Fields = append(e.Fields, discord.EmbedField{Name: "Vanity URL", Value: "discord.gg/" + *g.VanityURLCode, Inline: cmd.Ptr(true)})
	}
	if len(features) > 0 {
		e.Fields = append(e.Fields, discord.EmbedField{Name: "Features", Value: strings.Join(features, ", ")})
	}
	if icon := g.IconURL(discord.WithSize(256)); icon != nil {
		e.Thumbnail = &discord.EmbedResource{URL: *icon}
	}
	if banner := g.BannerURL(discord.WithSize(1024)); banner != nil {
		e.Image = &discord.EmbedResource{URL: *banner}
	}
	return c.ReplyEmbeds(e)
}

func verificationName(v discord.VerificationLevel) string {
	switch v {
	case discord.VerificationLevelNone:
		return "None"
	case discord.VerificationLevelLow:
		return "Low"
	case discord.VerificationLevelMedium:
		return "Medium"
	case discord.VerificationLevelHigh:
		return "High"
	case discord.VerificationLevelVeryHigh:
		return "Highest"
	}
	return "Unknown"
}

// FormatDuration renders a duration like "3d 4h 5m 6s".
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if h > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 || h > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	parts = append(parts, fmt.Sprintf("%ds", s))
	return strings.Join(parts, " ")
}

func stats(c *cmd.Context) error {
	st := c.Registry.Stats()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	var top strings.Builder
	for i, cc := range st.Top(5) {
		fmt.Fprintf(&top, "%d. `%s` × %d\n", i+1, cc.Name, cc.Count)
	}
	if top.Len() == 0 {
		top.WriteString("Nothing yet")
	}
	var allTime int64
	var allTop strings.Builder
	if counts, err := c.Registry.Store().CommandCounts(); err == nil {
		for i, cc := range counts {
			allTime += cc.Count
			if i < 5 {
				fmt.Fprintf(&allTop, "%d. `%s` × %d\n", i+1, cc.Name, cc.Count)
			}
		}
	}
	if allTop.Len() == 0 {
		allTop.WriteString("Nothing yet")
	}

	start := st.Start.Unix()
	e := discord.Embed{
		Title: "Bot statistics",
		Color: 0x5865F2,
		Fields: []discord.EmbedField{
			{Name: "Uptime", Value: fmt.Sprintf("%s\n(since <t:%d:f>)", FormatDuration(time.Since(st.Start)), start), Inline: cmd.Ptr(true)},
			{Name: "Servers", Value: fmt.Sprint(c.Client.Caches.GuildsLen()), Inline: cmd.Ptr(true)},
			{Name: "Latency", Value: c.Client.Gateway.Latency().Round(time.Millisecond).String(), Inline: cmd.Ptr(true)},
			{Name: "Commands this session", Value: fmt.Sprintf("%d total\n%d slash · %d prefix · %d menu\n%d failed · %d image jobs",
				st.Total(), st.Slash.Load(), st.Prefix.Load(), st.Menu.Load(), st.Errors.Load(), st.ImageJobs.Load()), Inline: cmd.Ptr(true)},
			{Name: "Commands all time", Value: fmt.Sprint(allTime), Inline: cmd.Ptr(true)},
			{Name: "Memory", Value: fmt.Sprintf("%.1f MiB heap\n%.1f MiB from OS", float64(mem.HeapAlloc)/(1<<20), float64(mem.Sys)/(1<<20)), Inline: cmd.Ptr(true)},
			{Name: "Top commands (session)", Value: top.String(), Inline: cmd.Ptr(true)},
			{Name: "Top commands (all time)", Value: allTop.String(), Inline: cmd.Ptr(true)},
		},
		Footer: &discord.EmbedFooter{Text: fmt.Sprintf("%s · disgo %s · %d goroutines", runtime.Version(), disgo.Version, runtime.NumGoroutine())},
	}
	return c.ReplyEmbeds(e)
}
