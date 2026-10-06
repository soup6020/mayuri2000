# Design

## Context

Commands are defined once in `internal/cmd` and serve both slash and text invocations. Text arguments are mapped to options by position in declaration order (`assign`). Typed options (Float) reject tokens that don't parse, and missing or invalid arguments already get a reply with the command's usage line. Slash commands list required options first; when all options are required, Discord keeps the declared order.

Autocomplete for string options already exists (used by `/time` and `/timezone`). It requires a String option without fixed choices.

## Goals / Non-Goals

**Goals:**
- Exact temperature conversion with the same argument order in both forms: `from`, `to`, `value`.
- A unit model that later units (length, mass, speed, …) can be added to without restructuring.

**Non-Goals:**
- Any units beyond Celsius, Fahrenheit and Kelvin in this change.
- Currency (it needs live rates), compound units or expressions like `5ft 3in`.
- Converting temperature *differences* (e.g. "a 10 °C rise") as opposed to absolute temperatures.

## Decisions

**Unit table by kind, converting through a base unit.**
A new package (`internal/units`) defines each unit with its kind, display symbol, accepted names, and conversion functions to and from the kind's base unit (kelvin for temperature). Converting is `to.fromBase(from.toBase(v))`, and units of different kinds are rejected. Temperature needs offsets as well as factors, so conversions are functions rather than plain factors. Linear units added later can use a small helper. Alternative considered: a pairwise conversion matrix, which grows quadratically and is error-prone.

**Absolute zero is checked in the base unit.**
After converting to kelvin, values below 0 are rejected (allowing for float error). So -300 °C, -500 °F and -1 K are all caught by one rule.

**Option order drives both forms.**
`/convert` declares `from` and `to` (String, required, autocompleted) and then `value` (Float, required), in that order. All three are required, so the slash command shows them in that order. The existing positional text mapping handles `%convert c f 30` with no custom parsing: `c` and `f` fill the unit options and `30` fills `value`, with negative and decimal values supported by the Float option. The framework's existing replies cover a missing argument (with usage `%convert <from> <to> <value>`) and a non-numeric value. Alternative considered: a free-form parser accepting forms like `100F to C`. It needed a new prefix-only option type and its own grammar, and the requested `from to value` order makes it unnecessary.

**Output formatting.**
Round to 2 decimals and trim trailing zeros. Show `-0` as `0`. Use the symbols `°C`, `°F` and `K` (kelvin has no degree sign), and bold the whole statement: `**100 °F = 37.78 °C**`. Inputs are echoed in the same formatting, so `%convert f c 100.000` shows `100 °F`. The magnitude limit of 1e12 keeps output readable without exponents.

**Autocomplete.**
`from` and `to` suggest the matching units as "Celsius (°C)" with value `celsius`, filtered by prefix on any accepted name. With no input they list all units. There are 3 units now, and the 25-choice limit applies as more are added.

**Placement.**
The command goes in the core module next to `/base64` (Tools category). The units package is pure and has no Discord dependencies, so it is fully unit-testable.

## Risks / Trade-offs

- [`unit` alias collides with a future command name] → Aliases are checked for duplicates at startup (registry panic), so a collision is caught immediately.
- [Float rounding, e.g. 37.77777…] → Two-decimal display. Tests pin the known reference points (-40 °F = -40 °C, 0 °C = 273.15 K, 212 °F = 100 °C).
- [Unit order mistakes, e.g. `%convert f c 30` when Celsius → Fahrenheit was meant] → The reply restates both units and values (**30 °F = -1.11 °C**), so a swapped order is obvious.
