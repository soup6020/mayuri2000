// Package core provides help, ping and prefix commands.
package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"

	"mayuri2000/internal/cmd"
)

func Commands() []*cmd.Command {
	return []*cmd.Command{
		{
			Name:        "ping",
			Category:    "Core",
			Description: "Check that the bot is alive",
			Run: func(c *cmd.Context) error {
				return c.Reply(fmt.Sprintf("Pong! Gateway latency: %s", c.Client.Gateway.Latency().Round(time.Millisecond)))
			},
		},
		{
			Name:        "help",
			Category:    "Core",
			Description: "List commands or show help for one",
			Options: []cmd.Option{
				{Name: "command", Description: "Command to show help for", Type: cmd.String},
			},
			Run: help,
		},
		{
			Name:        "prefix",
			Category:    "Core",
			Description: "Show or change this server's text command prefix",
			GuildOnly:   true,
			Options: []cmd.Option{
				{Name: "new", Description: "New prefix, or \"reset\" for the default", Type: cmd.String, MaxLength: cmd.Ptr(10)},
			},
			Run: prefix,
		},
		aliasCommand(),
		serverInfoCommand(),
		statsCommand(),
		timeCommand(),
		timezoneCommand(),
		userInfoCommand(),
		roleInfoCommand(),
		inRoleCommand(),
		base64Command(),
		convertCommand(),
	}
}

func help(c *cmd.Context) error {
	prefix := c.Registry.Prefix(c.GuildID)
	if name, ok := c.OptString("command"); ok {
		name = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, prefix)))
		first, _, _ := strings.Cut(name, " ")
		command, ok := c.Registry.Lookup(first)
		if !ok && c.GuildID != nil {
			if a, isAlias := c.Registry.Store().Alias(*c.GuildID, name); isAlias {
				return c.Reply(fmt.Sprintf("`%s%s` is an alias for `%s%s`.", prefix, a.Name, prefix, a.Command))
			}
		}
		if !ok {
			return cmd.Errorf("No command named `%s`.", name)
		}
		if len(command.Subcommands) > 0 {
			if _, subName, hasSub := strings.Cut(name, " "); hasSub {
				sub, found := command.Sub(strings.TrimSpace(subName))
				if !found {
					return cmd.Errorf("`%s` has no subcommand `%s`.", command.Name, strings.TrimSpace(subName))
				}
				command = sub
			} else {
				return c.ReplyEmbeds(discord.Embed{Title: "/" + command.Name, Description: cmd.GroupUsage(prefix, command)})
			}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n\n`%s`\n", command.Description, cmd.Usage(prefix, command))
		if len(command.Aliases) > 0 {
			fmt.Fprintf(&b, "Aliases: %s\n", strings.Join(command.Aliases, ", "))
		}
		for _, o := range command.Options {
			req := ""
			if o.Required {
				req = " (required)"
			}
			fmt.Fprintf(&b, "• `%s`%s: %s\n", o.Name, req, o.Description)
		}
		return c.ReplyEmbeds(discord.Embed{Title: "/" + command.FullName(), Description: b.String()})
	}

	var pages []cmd.Page
	var cur *discord.Embed
	for _, command := range c.Registry.Commands() {
		if cur == nil || cur.Title != command.Category {
			if cur != nil {
				pages = append(pages, cmd.Page{Embed: cur})
			}
			cur = &discord.Embed{
				Title:  command.Category,
				Footer: &discord.EmbedFooter{Text: fmt.Sprintf("Use /help <command> or %shelp <command> for details", prefix)},
			}
		}
		cur.Description += fmt.Sprintf("`%s%s` %s\n", prefix, command.Name, command.Description)
	}
	if cur != nil {
		pages = append(pages, cmd.Page{Embed: cur})
	}
	return c.Paginate(pages)
}

func prefix(c *cmd.Context) error {
	current := c.Registry.Prefix(c.GuildID)
	newPrefix, ok := c.OptString("new")
	if !ok {
		return c.Reply(fmt.Sprintf("The prefix here is `%s`. Slash commands work too.", current))
	}
	if !c.Permissions.Has(discord.PermissionManageGuild) {
		return cmd.Errorf("You need the Manage Server permission to change the prefix.")
	}
	newPrefix = strings.TrimSpace(newPrefix)
	if strings.EqualFold(newPrefix, "reset") {
		newPrefix = ""
	}
	if strings.ContainsAny(newPrefix, " \n\t") {
		return cmd.Errorf("The prefix can't contain spaces.")
	}
	if err := c.Registry.Store().SetPrefix(c, *c.GuildID, newPrefix); err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("Prefix set to `%s`.", c.Registry.Prefix(c.GuildID)))
}
