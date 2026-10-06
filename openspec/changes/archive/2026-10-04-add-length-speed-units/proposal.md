# Proposal

## Why

`/convert` only handles temperature. Distances (kilometres, miles, metres, feet) and speeds (km/h, mph) are the other conversions people in the servers reach for. The unit table in `internal/units` was built to grow by kind, so this is mostly new table entries.

## What Changes

- `/convert` (and `%convert`) gains two new unit kinds:
  - **Distance**: kilometres (`km`), miles (`mi`), metres (`m`) and feet (`ft`).
  - **Speed**: kilometres per hour (`km/h`) and miles per hour (`mph`).
- New accepted names, e.g. `km`, `kilometre(s)`, `kilometer(s)`; `mi`, `mile(s)`; `m`, `metre(s)`, `meter(s)`; `ft`, `foot`, `feet`; `kmh`, `km/h`, `kph`, `kmph`; `mph`.
- Display names use Canadian spelling (Kilometres, Metres); US spellings are still accepted as input.
- The reply format is unchanged, e.g. **10 km = 6.21 mi** and **100 km/h = 62.14 mph**.
- Conversions between kinds (e.g. km → °C, km → km/h) are rejected with the existing "different things" message.
- The absolute-zero check stays temperature-only. Negative distances and speeds are allowed and keep their sign.
- The command description, the `from`/`to` option descriptions, the unknown-unit list and autocomplete now include the new units.
- No commands are added or removed. Within Discord limits:
  - The description stays ≤100 characters.
  - Autocomplete returns 9 units, under the cap of 25.
  - Option count is unchanged.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `utilities`: the "Unit conversion", "Unit names" and "Conversion validation" requirements now cover the distance and speed units as well as temperature.

## Impact

- `internal/units/units.go`: adds the `Distance` and `Speed` kinds and six units. `Convert`, `Format` and `Statement` are unchanged.
- `internal/units/units_test.go`: adds lookup and conversion cases, and the mixed-kind test now uses real units.
- `internal/modules/core/convert.go`: description text only. Autocomplete already iterates `units.All`.
- `internal/modules/core/convert_test.go`, `cmd/mayuri/core_test.go`: new cases.
- README Tools list and the CLAUDE.md `internal/units` line.
- No new dependencies, storage or network access.
