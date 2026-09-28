package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

var nameRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// Validate checks registered commands against Discord's limits so problems
// surface at startup instead of as a failed command sync.
func (r *Registry) Validate() error {
	if len(r.commands) > 100 {
		return fmt.Errorf("%d slash commands registered; Discord allows 100", len(r.commands))
	}
	for _, c := range r.commands {
		if err := validateCommand(c); err != nil {
			return err
		}
		if len(c.Subcommands) == 0 {
			continue
		}
		if len(c.Options) > 0 || c.Run != nil {
			return fmt.Errorf("command %q: a group can't have its own options or Run", c.Name)
		}
		if len(c.Subcommands) > 25 {
			return fmt.Errorf("command %q: too many subcommands", c.Name)
		}
		seen := map[string]bool{}
		for _, sub := range c.Subcommands {
			if seen[sub.Name] {
				return fmt.Errorf("command %q: duplicate subcommand %q", c.Name, sub.Name)
			}
			seen[sub.Name] = true
			if len(sub.Subcommands) > 0 {
				return fmt.Errorf("command %q: nested subcommand groups aren't supported", sub.FullName())
			}
			if sub.Run == nil {
				return fmt.Errorf("command %q: missing Run", sub.FullName())
			}
			if err := validateCommand(sub); err != nil {
				return err
			}
		}
	}
	counts := map[MenuKind]int{}
	for _, m := range r.menus {
		counts[m.Kind]++
		if n := utf8.RuneCountInString(m.Name); n < 1 || n > 32 {
			return fmt.Errorf("menu %q: name must be 1-32 characters", m.Name)
		}
	}
	if counts[MenuMessage] > 5 || counts[MenuUser] > 5 {
		return errors.New("at most 5 message and 5 user context menus are allowed")
	}
	return nil
}

func validateCommand(c *Command) error {
	name := c.FullName()
	if !nameRe.MatchString(c.Name) {
		return fmt.Errorf("command %q: invalid name", name)
	}
	if n := utf8.RuneCountInString(c.Description); n < 1 || n > 100 {
		return fmt.Errorf("command %q: description must be 1-100 characters, got %d", name, n)
	}
	if len(c.Options) > 25 {
		return fmt.Errorf("command %q: too many options", name)
	}
	seen := map[string]bool{}
	for _, o := range c.Options {
		if !nameRe.MatchString(o.Name) {
			return fmt.Errorf("command %q: invalid option name %q", name, o.Name)
		}
		if seen[o.Name] {
			return fmt.Errorf("command %q: duplicate option %q", name, o.Name)
		}
		seen[o.Name] = true
		if n := utf8.RuneCountInString(o.Description); n < 1 || n > 100 {
			return fmt.Errorf("command %q option %q: description must be 1-100 characters", name, o.Name)
		}
		if o.Autocomplete != nil && (o.Type != String || len(o.Choices) > 0) {
			return fmt.Errorf("command %q option %q: autocomplete needs a String option without choices", name, o.Name)
		}
		if o.Rest && o.Type != String && o.Type != Role {
			return fmt.Errorf("command %q option %q: only string and role options can be Rest", name, o.Name)
		}
	}
	if len(c.Subcommands) == 0 && c.Run == nil {
		return fmt.Errorf("command %q: missing Run", name)
	}
	return nil
}
