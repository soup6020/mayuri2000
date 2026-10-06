package core

import (
	"slices"
	"testing"

	"mayuri2000/internal/units"
)

func TestUnitChoices(t *testing.T) {
	values := func(input string) []string {
		var out []string
		for _, c := range unitChoices(input) {
			out = append(out, c.Value)
		}
		return out
	}
	if got := values(""); len(got) != len(units.All) {
		t.Fatalf("empty input should list all units, got %v", got)
	}
	for input, want := range map[string]string{"c": "celsius", "Fahr": "fahrenheit", "°K": "kelvin", "kel": "kelvin", "cent": "celsius"} {
		got := values(input)
		if len(got) != 1 || got[0] != want {
			t.Errorf("unitChoices(%q) = %v, want [%s]", input, got, want)
		}
	}
	for input, want := range map[string][]string{"km": {"kilometre", "kmh"}, "fe": {"foot"}, "met": {"metre"}, "miles p": {"mph"}} {
		if got := values(input); !slices.Equal(got, want) {
			t.Errorf("unitChoices(%q) = %v, want %v", input, got, want)
		}
	}
	if got := values("x"); len(got) != 0 {
		t.Errorf("no unit starts with x, got %v", got)
	}
	if c := unitChoices("")[0]; c.Name != "Celsius (°C)" || len(c.Name) > 100 {
		t.Errorf("choice label %q", c.Name)
	}
	if n := len(unitChoices("")); n > 25 {
		t.Errorf("%d choices exceeds Discord's 25", n)
	}
}
