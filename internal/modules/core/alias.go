package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/store"
)

const maxAliases = 200

var aliasNameRe = regexp.MustCompile(`^[^\s]{1,32}$`)

func aliasCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "alias",
		Category:    "Core",
		Description: "Manage this server's text command aliases",
		GuildOnly:   true,
		Options: []cmd.Option{
			{Name: "action", Description: "What to do", Type: cmd.String, Required: true, Choices: []cmd.Choice{
				{Name: "add", Value: "add"}, {Name: "remove", Value: "remove"}, {Name: "list", Value: "list"}, {Name: "show", Value: "show"},
			}},
			{Name: "name", Description: "Alias name", Type: cmd.String, MaxLength: cmd.Ptr(32)},
			{Name: "command", Description: "Command (and leading arguments) the alias runs, e.g. \"magik 5\"", Type: cmd.String, Rest: true, MaxLength: cmd.Ptr(500)},
		},
		Run: alias,
	}
}

func alias(c *cmd.Context) error {
	st := c.Registry.Store()
	guild := *c.GuildID
	prefix := c.Registry.Prefix(c.GuildID)
	name := strings.ToLower(strings.TrimPrefix(c.String("name"), prefix))

	switch c.String("action") {
	case "list":
		aliases := st.Aliases(guild)
		if len(aliases) == 0 {
			return c.Reply(fmt.Sprintf("This server has no aliases. Add one with `%salias add <name> <command>`.", prefix))
		}
		var pages []cmd.Page
		for i := 0; i < len(aliases); i += 20 {
			var b strings.Builder
			for _, a := range aliases[i:min(i+20, len(aliases))] {
				fmt.Fprintf(&b, "`%s%s` → `%s%s`\n", prefix, a.Name, prefix, a.Command)
			}
			pages = append(pages, cmd.Page{Embed: &discord.Embed{
				Title:       fmt.Sprintf("Aliases (%d)", len(aliases)),
				Description: b.String(),
				Footer:      &discord.EmbedFooter{Text: "Aliases work with the text prefix only"},
			}})
		}
		return c.Paginate(pages)

	case "show":
		a, ok := st.Alias(guild, name)
		if !ok {
			return cmd.Errorf("There's no alias named `%s`.", name)
		}
		return c.Reply(fmt.Sprintf("`%s%s` → `%s%s` (created by <@%s> <t:%d:R>)", prefix, a.Name, prefix, a.Command, a.CreatorID, a.CreatedAt.Unix()))

	case "add":
		if !c.Permissions.Has(discord.PermissionManageGuild) {
			return cmd.Errorf("You need the Manage Server permission to add aliases.")
		}
		if !aliasNameRe.MatchString(name) {
			return cmd.Errorf("Alias names must be 1-32 characters with no spaces.")
		}
		if _, exists := c.Registry.Lookup(name); exists {
			return cmd.Errorf("`%s` is already a command.", name)
		}
		command := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.String("command")), prefix))
		if command == "" {
			return cmd.Errorf("Tell me which command the alias should run, e.g. `%salias add %s magik 5`.", prefix, name)
		}
		target, rest, _ := strings.Cut(command, " ")
		targetCmd, ok := c.Registry.Lookup(target)
		if !ok {
			if _, isAlias := st.Alias(guild, strings.ToLower(target)); isAlias {
				return cmd.Errorf("Aliases can't point to other aliases.")
			}
			return cmd.Errorf("There's no command named `%s`.", target)
		}
		// Store the canonical command name so renamed aliases keep working.
		command = strings.TrimSpace(targetCmd.Name + " " + rest)
		_, replacing := st.Alias(guild, name)
		if !replacing && len(st.Aliases(guild)) >= maxAliases {
			return cmd.Errorf("This server already has %d aliases, the maximum.", maxAliases)
		}
		err := st.SetAlias(guild, store.Alias{Name: name, Command: command, CreatorID: c.Author.ID, CreatedAt: time.Now()})
		if err != nil {
			return err
		}
		verb := "Added"
		if replacing {
			verb = "Updated"
		}
		return c.Reply(fmt.Sprintf("%s alias `%s%s` → `%s%s`.", verb, prefix, name, prefix, command))

	case "remove":
		if !c.Permissions.Has(discord.PermissionManageGuild) {
			return cmd.Errorf("You need the Manage Server permission to remove aliases.")
		}
		removed, err := st.DeleteAlias(guild, name)
		if err != nil {
			return err
		}
		if !removed {
			return cmd.Errorf("There's no alias named `%s`.", name)
		}
		return c.Reply(fmt.Sprintf("Removed alias `%s%s`.", prefix, name))
	}
	return cmd.Errorf("Unknown action. Use add, remove, list or show.")
}
