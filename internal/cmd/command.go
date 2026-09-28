// Package cmd implements a small command framework where each command is
// defined once and can be invoked either as a slash command or through a
// text prefix.
package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/omit"
)

type OptionType int

const (
	String OptionType = iota
	Int
	Bool
	User
	Attachment
	Float
	// Role accepts a role mention, ID or (for prefix commands) name.
	Role
	// Channel accepts a text channel or thread mention, ID or (for prefix
	// commands) name. Its value is the channel's snowflake.ID.
	Channel
)

type Choice struct {
	Name  string
	Value string
}

type Option struct {
	Name        string
	Description string
	Type        OptionType
	Required    bool
	// Rest makes a prefix invocation consume the remaining raw text for this
	// option. Only valid for the last String or Role option.
	Rest bool
	// SlashOnly options are never filled from prefix arguments.
	SlashOnly bool
	// Accept is consulted for prefix invocations of optional options: when it
	// returns false the token is left for the next option instead.
	Accept    func(tok string) bool
	Min, Max  *int
	MaxLength *int
	Choices   []Choice
	// Autocomplete suggests values for a String option as the user types a
	// slash command. It returns at most 25 choices.
	Autocomplete func(input string) []Choice
}

type Command struct {
	Name        string
	Aliases     []string
	Category    string
	Description string
	Options     []Option
	// Perms are required of the invoking member. They are also used as the
	// slash command's default member permissions.
	Perms     discord.Permissions
	GuildOnly bool
	// Heavy commands are deferred immediately and serialised per guild.
	Heavy bool
	Run   func(*Context) error

	// Subcommands turn this command into a group: /name sub ... for slash
	// commands and "prefix name sub ..." for text. A group has no Options or
	// Run of its own; its Perms and GuildOnly apply to every subcommand.
	Subcommands []*Command
	parent      *Command
}

// FullName is the command's name including its group, e.g. "rss add".
func (c *Command) FullName() string {
	if c.parent != nil {
		return c.parent.Name + " " + c.Name
	}
	return c.Name
}

// Sub finds a subcommand by name or alias.
func (c *Command) Sub(name string) (*Command, bool) {
	name = strings.ToLower(name)
	for _, s := range c.Subcommands {
		if s.Name == name || slices.Contains(s.Aliases, name) {
			return s, true
		}
	}
	return nil, false
}

// effective returns the permissions, guild restriction and weight of a leaf
// command, merged with its group's.
func (c *Command) effective() (discord.Permissions, bool, bool) {
	perms, guildOnly := c.Perms, c.GuildOnly
	if c.parent != nil {
		perms |= c.parent.Perms
		guildOnly = guildOnly || c.parent.GuildOnly
	}
	return perms, guildOnly, c.Heavy
}

type MenuKind int

const (
	MenuMessage MenuKind = iota
	MenuUser
)

// Menu is a context menu command (right-click on a message or user).
type Menu struct {
	Name  string
	Kind  MenuKind
	Heavy bool
	Run   func(*Context) error
}

// UserError is an error whose message is safe to show to the user.
type UserError struct{ msg string }

func (e *UserError) Error() string { return e.msg }

// Errorf returns an error that is shown verbatim to the invoking user.
func Errorf(format string, args ...any) error {
	return &UserError{msg: fmt.Sprintf(format, args...)}
}

func userMessage(err error) (string, bool) {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.msg, true
	}
	return "", false
}

func Ptr[T any](v T) *T { return &v }

func (c *Command) slashCreate() discord.SlashCommandCreate {
	sc := discord.SlashCommandCreate{
		Name:        c.Name,
		Description: c.Description,
	}
	if c.GuildOnly {
		sc.Contexts = []discord.InteractionContextType{discord.InteractionContextTypeGuild}
	} else {
		sc.Contexts = []discord.InteractionContextType{discord.InteractionContextTypeGuild, discord.InteractionContextTypeBotDM}
	}
	if c.Perms != 0 {
		sc.DefaultMemberPermissions = omit.NewPtr(c.Perms)
	}
	if len(c.Subcommands) > 0 {
		for _, s := range c.Subcommands {
			sc.Options = append(sc.Options, discord.ApplicationCommandOptionSubCommand{
				Name: s.Name, Description: s.Description, Options: slashOptions(s.Options),
			})
		}
		return sc
	}
	sc.Options = slashOptions(c.Options)
	return sc
}

// slashOptions converts options, putting required ones first as Discord
// requires.
func slashOptions(opts []Option) []discord.ApplicationCommandOption {
	var out []discord.ApplicationCommandOption
	for _, required := range []bool{true, false} {
		for _, o := range opts {
			if o.Required == required {
				out = append(out, o.slashOption())
			}
		}
	}
	return out
}

func (o Option) slashOption() discord.ApplicationCommandOption {
	switch o.Type {
	case Int:
		opt := discord.ApplicationCommandOptionInt{Name: o.Name, Description: o.Description, Required: o.Required, MinValue: o.Min, MaxValue: o.Max}
		return opt
	case Float:
		opt := discord.ApplicationCommandOptionFloat{Name: o.Name, Description: o.Description, Required: o.Required}
		if o.Min != nil {
			opt.MinValue = Ptr(float64(*o.Min))
		}
		if o.Max != nil {
			opt.MaxValue = Ptr(float64(*o.Max))
		}
		return opt
	case Bool:
		return discord.ApplicationCommandOptionBool{Name: o.Name, Description: o.Description, Required: o.Required}
	case User:
		return discord.ApplicationCommandOptionUser{Name: o.Name, Description: o.Description, Required: o.Required}
	case Attachment:
		return discord.ApplicationCommandOptionAttachment{Name: o.Name, Description: o.Description, Required: o.Required}
	case Role:
		return discord.ApplicationCommandOptionRole{Name: o.Name, Description: o.Description, Required: o.Required}
	case Channel:
		return discord.ApplicationCommandOptionChannel{Name: o.Name, Description: o.Description, Required: o.Required, ChannelTypes: TextChannelTypes}
	default:
		opt := discord.ApplicationCommandOptionString{Name: o.Name, Description: o.Description, Required: o.Required, MaxLength: o.MaxLength, Autocomplete: o.Autocomplete != nil}
		for _, c := range o.Choices {
			opt.Choices = append(opt.Choices, discord.ApplicationCommandOptionChoiceString{Name: c.Name, Value: c.Value})
		}
		return opt
	}
}

func (m *Menu) create() discord.ApplicationCommandCreate {
	contexts := []discord.InteractionContextType{discord.InteractionContextTypeGuild, discord.InteractionContextTypeBotDM}
	if m.Kind == MenuUser {
		return discord.UserCommandCreate{Name: m.Name, Contexts: contexts}
	}
	return discord.MessageCommandCreate{Name: m.Name, Contexts: contexts}
}

// TextChannelTypes are the channel types messages can be posted in.
var TextChannelTypes = []discord.ChannelType{
	discord.ChannelTypeGuildText,
	discord.ChannelTypeGuildNews,
	discord.ChannelTypeGuildPublicThread,
	discord.ChannelTypeGuildPrivateThread,
	discord.ChannelTypeGuildNewsThread,
}
