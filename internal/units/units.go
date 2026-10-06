// Package units converts values between units of the same kind
// (temperature, distance, speed). Each unit converts to and from its kind's base unit, so
// adding a unit never needs pairwise conversion code.
package units

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Kind groups units that can be converted into each other.
type Kind string

const (
	Temperature Kind = "temperature"
	Distance    Kind = "distance" // base: metre
	Speed       Kind = "speed"    // base: metre per second
)

// MaxMagnitude bounds input values so results stay readable.
const MaxMagnitude = 1e12

// Unit is a convertible unit.
type Unit struct {
	ID     string // canonical lowercase name, e.g. "celsius"
	Name   string // display name, e.g. "Celsius"
	Symbol string // e.g. "°C"
	Kind   Kind
	// Names are accepted spellings, lowercase.
	Names    []string
	toBase   func(float64) float64
	fromBase func(float64) float64
}

// linear builds a unit whose base value is v*perUnit.
func linear(id, name, symbol string, kind Kind, perUnit float64, names ...string) Unit {
	return Unit{
		ID: id, Name: name, Symbol: symbol, Kind: kind, Names: names,
		toBase:   func(v float64) float64 { return v * perUnit },
		fromBase: func(b float64) float64 { return b / perUnit },
	}
}

// All lists the supported units in display order. Display names use
// Canadian spelling; US spellings are accepted as names.
var All = []Unit{
	{
		ID: "celsius", Name: "Celsius", Symbol: "°C", Kind: Temperature,
		Names:    []string{"c", "°c", "℃", "celsius", "centigrade", "degc"},
		toBase:   func(v float64) float64 { return v + 273.15 },
		fromBase: func(k float64) float64 { return k - 273.15 },
	},
	{
		ID: "fahrenheit", Name: "Fahrenheit", Symbol: "°F", Kind: Temperature,
		Names:    []string{"f", "°f", "℉", "fahrenheit", "degf"},
		toBase:   func(v float64) float64 { return (v + 459.67) * 5 / 9 },
		fromBase: func(k float64) float64 { return k*9/5 - 459.67 },
	},
	{
		ID: "kelvin", Name: "Kelvin", Symbol: "K", Kind: Temperature,
		Names:    []string{"k", "°k", "kelvin", "kelvins"},
		toBase:   func(v float64) float64 { return v },
		fromBase: func(k float64) float64 { return k },
	},
	linear("kilometre", "Kilometres", "km", Distance, 1000,
		"km", "kms", "kilometre", "kilometres", "kilometer", "kilometers"),
	linear("mile", "Miles", "mi", Distance, 1609.344, "mi", "mile", "miles"),
	linear("metre", "Metres", "m", Distance, 1, "m", "metre", "metres", "meter", "meters"),
	linear("foot", "Feet", "ft", Distance, 0.3048, "ft", "foot", "feet"),
	linear("kmh", "Kilometres per hour", "km/h", Speed, 1/3.6,
		"km/h", "kmh", "kph", "kmph", "km/hr", "kilometres per hour", "kilometers per hour"),
	linear("mph", "Miles per hour", "mph", Speed, 0.44704, "mph", "mi/h", "miles per hour"),
}

var byName = func() map[string]Unit {
	m := map[string]Unit{}
	for _, u := range All {
		for _, n := range u.Names {
			m[n] = u
		}
	}
	return m
}()

// UnknownUnitError is returned for unsupported unit names.
type UnknownUnitError struct{ Name string }

func (e *UnknownUnitError) Error() string {
	return fmt.Sprintf("`%s` isn't a supported unit. Supported units: %s", e.Name, SupportedList())
}

// SupportedList names every unit, e.g. "Celsius (°C), Fahrenheit (°F), Kelvin (K)".
func SupportedList() string {
	names := make([]string, len(All))
	for i, u := range All {
		names[i] = fmt.Sprintf("%s (%s)", u.Name, u.Symbol)
	}
	return strings.Join(names, ", ")
}

// Lookup finds a unit by any accepted name, case-insensitively, ignoring
// surrounding spaces and a "degree(s)" prefix.
func Lookup(name string) (Unit, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(n, "degrees"), "degree"))
	if u, ok := byName[n]; ok {
		return u, nil
	}
	return Unit{}, &UnknownUnitError{Name: strings.TrimSpace(name)}
}

var (
	ErrNotFinite      = errors.New("that isn't a finite number")
	ErrTooLarge       = fmt.Errorf("values must be between -%s and %s", Format(MaxMagnitude), Format(MaxMagnitude))
	ErrBelowAbsZero   = errors.New("that temperature is below absolute zero (0 K, -273.15 °C, -459.67 °F)")
	ErrDifferentKinds = errors.New("those units measure different things and can't be converted")
)

// absZeroTolerance absorbs float error so exactly -273.15 °C is accepted.
const absZeroTolerance = 1e-9

// Convert converts v from one unit to another of the same kind.
func Convert(v float64, from, to Unit) (float64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, ErrNotFinite
	}
	if math.Abs(v) > MaxMagnitude {
		return 0, ErrTooLarge
	}
	if from.Kind != to.Kind {
		return 0, ErrDifferentKinds
	}
	base := from.toBase(v)
	if from.Kind == Temperature && base < -absZeroTolerance {
		return 0, ErrBelowAbsZero
	}
	if from.ID == to.ID {
		return v, nil
	}
	return to.fromBase(base), nil
}

// Format renders v with at most 2 decimals and no trailing zeros; -0 is 0.
func Format(v float64) string {
	v = math.Round(v*100) / 100
	if v == 0 {
		v = 0 // normalise -0
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}

// Statement renders a conversion, e.g. "**100 °F = 37.78 °C**".
func Statement(v float64, from Unit, result float64, to Unit) string {
	return fmt.Sprintf("**%s %s = %s %s**", Format(v), from.Symbol, Format(result), to.Symbol)
}
