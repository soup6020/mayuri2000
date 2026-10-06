package core

import (
	"strings"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/units"
)

func convertCommand() *cmd.Command {
	// Options are declared from, to, value: text arguments map positionally
	// in this order ("%convert c f 30"), and the all-required slash options
	// keep it too.
	return &cmd.Command{
		Name:        "convert",
		Aliases:     []string{"conv", "unit"},
		Category:    "Tools",
		Description: "Convert between units of temperature, distance or speed",
		Options: []cmd.Option{
			{Name: "from", Description: "Unit to convert from, e.g. C, km or mph", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(32), Autocomplete: unitChoices},
			{Name: "to", Description: "Unit to convert to, e.g. F, mi or km/h", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(32), Autocomplete: unitChoices},
			{Name: "value", Description: "Value to convert", Type: cmd.Float, Required: true},
		},
		Run: convert,
	}
}

// unitChoices suggests units whose display name or any accepted name starts
// with the input; empty input lists every unit.
func unitChoices(input string) []cmd.Choice {
	q := strings.ToLower(strings.TrimSpace(input))
	var out []cmd.Choice
	for _, u := range units.All {
		match := q == "" || strings.HasPrefix(strings.ToLower(u.Name), q)
		for _, n := range u.Names {
			match = match || strings.HasPrefix(n, q)
		}
		if match && len(out) < 25 {
			out = append(out, cmd.Choice{Name: u.Name + " (" + u.Symbol + ")", Value: u.ID})
		}
	}
	return out
}

func convert(c *cmd.Context) error {
	from, err := units.Lookup(c.String("from"))
	if err != nil {
		return cmd.Errorf("%s", err)
	}
	to, err := units.Lookup(c.String("to"))
	if err != nil {
		return cmd.Errorf("%s", err)
	}
	value := c.Float("value")
	result, err := units.Convert(value, from, to)
	if err != nil {
		return cmd.Errorf("%s", capitalize(err.Error()))
	}
	return c.Reply(units.Statement(value, from, result, to))
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
