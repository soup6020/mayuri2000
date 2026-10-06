package units

import (
	"errors"
	"math"
	"testing"
)

func mustLookup(t *testing.T, name string) Unit {
	t.Helper()
	u, err := Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLookup(t *testing.T) {
	for name, want := range map[string]string{
		"c": "celsius", "C": "celsius", "°C": "celsius", "℃": "celsius", "Celsius": "celsius", "centigrade": "celsius",
		"f": "fahrenheit", "°F": "fahrenheit", "℉": "fahrenheit", "FAHRENHEIT": "fahrenheit",
		"k": "kelvin", "°K": "kelvin", "Kelvin": "kelvin", "kelvins": "kelvin",
		"km": "kilometre", "KM": "kilometre", "kilometres": "kilometre", "Kilometers": "kilometre",
		"mi": "mile", "Miles": "mile", "m": "metre", "metres": "metre", "meters": "metre",
		"ft": "foot", "foot": "foot", "Feet": "foot",
		"km/h": "kmh", "kmh": "kmh", "KPH": "kmh", "kmph": "kmh", "kilometres per hour": "kmh",
		"mph": "mph", "MPH": "mph", "mi/h": "mph", "miles per hour": "mph",
	} {
		if u, err := Lookup(name); err != nil || u.ID != want {
			t.Errorf("Lookup(%q) = %v, %v; want %s", name, u.ID, err, want)
		}
	}
	// Surrounding spaces and a "degrees" prefix are ignored.
	if u, err := Lookup("  degrees C "); err != nil || u.ID != "celsius" {
		t.Errorf("Lookup with degrees prefix = %v, %v", u.ID, err)
	}
	_, err := Lookup("rankine")
	var unknown *UnknownUnitError
	if !errors.As(err, &unknown) || unknown.Error() != "`rankine` isn't a supported unit. Supported units: Celsius (°C), Fahrenheit (°F), Kelvin (K), Kilometres (km), Miles (mi), Metres (m), Feet (ft), Kilometres per hour (km/h), Miles per hour (mph)" {
		t.Fatalf("unknown unit error: %v", err)
	}
}

func TestConvert(t *testing.T) {
	cases := []struct {
		v        float64
		from, to string
		want     float64
	}{
		{-40, "f", "c", -40},
		{-40, "c", "f", -40},
		{0, "c", "k", 273.15},
		{273.15, "k", "c", 0},
		{212, "f", "c", 100},
		{100, "c", "f", 212},
		{100, "f", "c", 37.7777777},
		{30, "c", "f", 86},
		{0, "k", "f", -459.67},
		{-273.15, "c", "k", 0},
		{-459.67, "f", "k", 0},
		{37.5, "c", "c", 37.5}, // identity
		{10, "km", "mi", 6.2137119},
		{26.2, "mi", "km", 42.1648128},
		{100, "ft", "m", 30.48},
		{1, "m", "ft", 3.28083990},
		{1, "km", "ft", 3280.839895},
		{-1, "mi", "km", -1.609344}, // negative distances keep their sign
		{3.5, "mi", "mi", 3.5},
		{100, "kmh", "mph", 62.1371192},
		{60, "mph", "km/h", 96.560640},
		{-10, "mph", "kph", -16.09344},
	}
	for _, c := range cases {
		got, err := Convert(c.v, mustLookup(t, c.from), mustLookup(t, c.to))
		if err != nil || math.Abs(got-c.want) > 1e-6 {
			t.Errorf("Convert(%v %s → %s) = %v, %v; want %v", c.v, c.from, c.to, got, err, c.want)
		}
	}
}

func TestConvertRejects(t *testing.T) {
	c, f, k := mustLookup(t, "c"), mustLookup(t, "f"), mustLookup(t, "k")
	km, mph := mustLookup(t, "km"), mustLookup(t, "mph")
	for _, tc := range []struct {
		v        float64
		from, to Unit
		want     error
	}{
		{-300, c, f, ErrBelowAbsZero},
		{-500, f, c, ErrBelowAbsZero},
		{-1, k, c, ErrBelowAbsZero},
		{-273.16, c, c, ErrBelowAbsZero}, // identity still validated
		{math.NaN(), c, f, ErrNotFinite},
		{math.Inf(1), c, f, ErrNotFinite},
		{2e12, c, f, ErrTooLarge},
		{-2e12, f, c, ErrTooLarge},
		{1, km, c, ErrDifferentKinds},
		{1, c, km, ErrDifferentKinds},
		{1, km, mph, ErrDifferentKinds},
	} {
		if _, err := Convert(tc.v, tc.from, tc.to); !errors.Is(err, tc.want) {
			t.Errorf("Convert(%v %s → %s) error = %v, want %v", tc.v, tc.from.ID, tc.to.ID, err, tc.want)
		}
	}
}

func TestFormat(t *testing.T) {
	for v, want := range map[float64]string{
		37.7777777: "37.78",
		100.000:    "100",
		273.15:     "273.15",
		-40:        "-40",
		0.1:        "0.1",
		-0.001:     "0",
		1e12:       "1000000000000",
		-1.115:     "-1.12",
	} {
		if got := Format(v); got != want {
			t.Errorf("Format(%v) = %q, want %q", v, got, want)
		}
	}
	if got := Format(math.Copysign(0, -1)); got != "0" {
		t.Errorf("Format(-0) = %q, want 0", got)
	}
}

func TestStatement(t *testing.T) {
	f, c, k := mustLookup(t, "f"), mustLookup(t, "c"), mustLookup(t, "k")
	r, _ := Convert(100, f, c)
	if got := Statement(100, f, r, c); got != "**100 °F = 37.78 °C**" {
		t.Fatalf("got %q", got)
	}
	r, _ = Convert(0, c, k)
	if got := Statement(0, c, r, k); got != "**0 °C = 273.15 K**" {
		t.Fatalf("got %q", got)
	}
}
