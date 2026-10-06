# Design

## Context

`internal/units` stores each unit as a row with `Kind`, accepted `Names`, and `toBase`/`fromBase` functions. `Convert` already rejects mismatched kinds, and it applies the absolute-zero check only when `from.Kind == Temperature`. Autocomplete (`unitChoices`), `SupportedList` and the unknown-unit error all iterate `units.All`, so new rows show up in each of them automatically.

## Goals / Non-Goals

**Goals:**
- Add six units by adding table rows only. `Convert`, `Format` and `Statement` stay as they are.

**Non-Goals:**
- Other distance units (centimetres, inches, yards, nautical miles) and other speed units (m/s, knots). The table makes them easy to add later.
- Autocomplete that narrows `to` based on the `from` already chosen. With 9 units the full list is short enough.

## Decisions

- **New kinds `Distance` (base: metre) and `Speed` (base: metre per second).**
  - A real SI base makes future units one row each.
  - Factors are exact:
    - 1 km = 1000 m.
    - 1 mi = 1609.344 m (international mile).
    - 1 ft = 0.3048 m (international foot).
    - 1 km/h = 1/3.6 m/s.
    - 1 mph = 0.44704 m/s.
  - Alternative considered: a base of km and km/h. It's simpler today but makes later units awkward.
- **Display names and symbols:** `Kilometres (km)`, `Miles (mi)`, `Metres (m)`, `Feet (ft)`, `Kilometres per hour (km/h)`, `Miles per hour (mph)`.
  - Canadian spelling for display, as requested. US "kilometer(s)"/"meter(s)" are still accepted as input.
  - Bare `m` means metres, the standard SI symbol; miles are `mi`.
  - Table order is temperature, then distance, then speed. That order is the autocomplete order and the supported-units list order.
- **Accepted names:**
  - Distance: `km`, `kms`, `kilometre(s)`, `kilometer(s)`, `mi`, `mile(s)`, `m`, `metre(s)`, `meter(s)`, `ft`, `foot`, `feet`.
  - Speed: `km/h`, `kmh`, `kph`, `kmph`, `km/hr`, `kilometers per hour`, `kilometres per hour`, `mph`, `mi/h`, `miles per hour`.
  - Multi-word names work in slash options. In text form they need quotes, because the tokenizer splits on spaces.
  - Lookup strips a `degree(s)` prefix. None of the new names starts with `degree`, so there's no collision.
- **Negative values are allowed for distance and speed.** The sign is kept, which matches how the value is passed through today and avoids inventing new validation. Only temperature has a physical floor.
- **The error message for mixed kinds stays as it is** ("those units measure different things…"). It already reads correctly for km → mph.
- **Command description:** "Convert between units of temperature, distance or speed" (55 chars, under the 100 limit). Option descriptions become e.g. "Unit to convert from, e.g. C, km or mph".

## Risks / Trade-offs

- [The unknown-unit message gets longer: 9 units, about 190 chars] → It stays well under the message limit, and the existing test still finds "Kelvin (K)" in it.
- [Float rounding, e.g. 1 mi → 1.609344 km displays as 1.61] → Accepted. Results are always shown to 2 decimals.
- [`k` (Kelvin) vs. `km`/`kmh`] → Lookup matches exact names, never prefixes, so these can't be confused. Autocomplete for `k` lists Kelvin and the km units, which is fine.
