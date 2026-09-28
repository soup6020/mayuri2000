package cmd

import (
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
		if !nameRe.MatchString(c.Name) {
			return fmt.Errorf("command %q: invalid name", c.Name)
		}
		if n := utf8.RuneCountInString(c.Description); n < 1 || n > 100 {
			return fmt.Errorf("command %q: description must be 1-100 characters, got %d", c.Name, n)
		}
		if len(c.Options) > 25 {
			return fmt.Errorf("command %q: too many options", c.Name)
		}
		seen := map[string]bool{}
		for _, o := range c.Options {
			if !nameRe.MatchString(o.Name) {
				return fmt.Errorf("command %q: invalid option name %q", c.Name, o.Name)
			}
			if seen[o.Name] {
				return fmt.Errorf("command %q: duplicate option %q", c.Name, o.Name)
			}
			seen[o.Name] = true
			if n := utf8.RuneCountInString(o.Description); n < 1 || n > 100 {
				return fmt.Errorf("command %q option %q: description must be 1-100 characters", c.Name, o.Name)
			}
			if o.Rest && o.Type != String {
				return fmt.Errorf("command %q option %q: only string options can be Rest", c.Name, o.Name)
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
		return fmt.Errorf("Discord allows at most 5 message and 5 user context menus")
	}
	return nil
}
