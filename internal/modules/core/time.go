package core

import (
	"fmt"
	"strings"
	"time"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/tz"
)

func timezoneCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "timezone",
		Aliases:     []string{"settimezone", "mytz"},
		Category:    "Info",
		Description: "Show or set your time zone, used by /time and /remind",
		Options: []cmd.Option{{
			Name:         "zone",
			Description:  "IANA time zone, e.g. Europe/London; \"reset\" to clear",
			Type:         cmd.String,
			Rest:         true,
			MaxLength:    cmd.Ptr(64),
			Autocomplete: zoneChoices,
		}},
		Run: setTimezone,
	}
}

func setTimezone(c *cmd.Context) error {
	st := c.Registry.Store()
	query, ok := c.OptString("zone")
	if !ok || strings.TrimSpace(query) == "" {
		current, has, err := st.UserTimezone(c, c.Author.ID)
		if err != nil {
			return err
		}
		if !has {
			return c.ReplyEphemeral("You haven't set a time zone, so times are read as UTC. Set one with `/timezone zone:Europe/London`.")
		}
		_, loc, _ := tz.Lookup(current)
		return c.ReplyEphemeral(fmt.Sprintf("Your time zone is `%s`. It's %s there.", current, FormatTime(time.Now().In(loc))))
	}
	if strings.EqualFold(strings.TrimSpace(query), "reset") {
		if err := st.SetUserTimezone(c, c.Author.ID, ""); err != nil {
			return err
		}
		return c.ReplyEphemeral("Cleared your time zone; times will be read as UTC.")
	}
	name, loc, ok := tz.Lookup(query)
	if !ok {
		return unknownZone(query)
	}
	if err := st.SetUserTimezone(c, c.Author.ID, name); err != nil {
		return err
	}
	return c.ReplyEphemeral(fmt.Sprintf("Your time zone is now `%s`. It's %s there.", name, FormatTime(time.Now().In(loc))))
}

func unknownZone(query string) error {
	suggestions := tz.Search(query, 5)
	if len(suggestions) == 0 {
		return cmd.Errorf("I don't know the time zone `%s`. Use an IANA name like `Europe/London` or `America/New_York`.", query)
	}
	return cmd.Errorf("I don't know the time zone `%s`. Did you mean: %s?", query, "`"+strings.Join(suggestions, "`, `")+"`")
}

func timeCommand() *cmd.Command {
	return &cmd.Command{
		Name:        "time",
		Aliases:     []string{"clock", "tz"},
		Category:    "Info",
		Description: "Show the current time in a time zone (IANA names, e.g. Europe/London)",
		Options: []cmd.Option{{
			Name:         "zone",
			Description:  "IANA time zone, e.g. America/New_York (default: yours, or UTC)",
			Type:         cmd.String,
			Rest:         true,
			MaxLength:    cmd.Ptr(64),
			Autocomplete: zoneChoices,
		}},
		Run: showTime,
	}
}

func zoneChoices(input string) []cmd.Choice {
	var out []cmd.Choice
	for _, name := range tz.Search(input, 25) {
		out = append(out, cmd.Choice{Name: name, Value: name})
	}
	return out
}

// FormatTime renders t in its location, e.g.
// "**14:05:09** (2:05 PM) · Tuesday, 3 March 2026 · CET (UTC+01:00)".
func FormatTime(t time.Time) string {
	abbr, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	utc := fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, offset%3600/60)
	zone := utc
	// Zones without a real abbreviation report a numeric one like "+0530".
	if abbr != "" && !strings.ContainsAny(abbr[:1], "+-") && abbr != "UTC" {
		zone = fmt.Sprintf("%s (%s)", abbr, utc)
	}
	return fmt.Sprintf("**%s** (%s) · %s · %s", t.Format("15:04:05"), t.Format("3:04 PM"), t.Format("Monday, 2 January 2006"), zone)
}

func showTime(c *cmd.Context) error {
	query := strings.TrimSpace(c.String("zone"))
	if query == "" {
		query = "UTC"
		if saved, ok, err := c.Registry.Store().UserTimezone(c, c.Author.ID); err == nil && ok {
			query = saved
		}
	}
	name, loc, ok := tz.Lookup(query)
	if !ok {
		return unknownZone(query)
	}
	now := time.Now()
	return c.Reply(fmt.Sprintf("🕒 `%s`: %s\n-# Your local time: <t:%d:F>", name, FormatTime(now.In(loc)), now.Unix()))
}
