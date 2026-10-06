# Tasks

## 1. Units package

- [x] 1.1 Add `Distance` (base metre) and `Speed` (base m/s) kinds and the kilometre, mile, metre, foot, km/h and mph rows (Canadian display spelling) to `units.All` (after the temperature units) with the accepted names from design.md and exact factors (1609.344 m/mi, 0.3048 m/ft, 0.44704 m/s per mph, 1/3.6 m/s per km/h); verify with `TestLookup` cases for every new name (incl. `KPH`, `kilometers`, `meters`, `feet`, `miles per hour`) and an updated unknown-unit message listing all 9 units
- [x] 1.2 Extend `TestConvert` with 10 km → 6.2137 mi, 26.2 mi → 42.1648 km, 100 ft → 30.48 m, 1 m → 3.28084 ft, 1 km → 3280.84 ft, 100 km/h → 62.137 mph, 60 mph → 96.5606 km/h, -1 mi → -1.609344 km and identity; switch the mixed-kind case in `TestConvertRejects` to real units (km → °C, km → mph) and confirm negative distance/speed is not rejected; verify `go test ./internal/units/` passes

## 2. Command

- [x] 2.1 Update the `/convert` description to "Convert between units of temperature, distance or speed" and the `from`/`to` option descriptions to mention km/mph (e.g. "Unit to convert from, e.g. C, km or mph"); verify `TestCommandsValid` passes (≤100-char descriptions)
- [x] 2.2 Extend `TestUnitChoices` (`km` → kilometres and km/h, `fe` → feet, `met` → metres, `mph` → miles per hour, empty input lists all 9, ≤25 choices) and `TestConvertCommand` with `%convert km mi 10` → **10 km = 6.21 mi**, `%convert kmh mph 100` → **100 km/h = 62.14 mph**, `%convert mph km/h 60` → **60 mph = 96.56 km/h**, `%convert ft m 100` → **100 ft = 30.48 m**, `%convert mi km -1` → **-1 mi = -1.61 km**, and `%convert km mph 10` reporting different kinds; verify `go test ./...` passes

## 3. Docs and checks

- [x] 3.1 Update the README Tools entry for `convert` (temperature, distance, speed; add a `%convert km mi 10` example) and the CLAUDE.md `internal/units` line; verify by reading both sections
- [x] 3.2 Run `golangci-lint run ./...` (0 issues), `openspec validate --specs --strict` and `nix flake check`; verify all pass
- [x] 3.3 Manually: in a test server run `/convert` with autocomplete for `km`, `%convert km mi 10`, `%convert ft m 100` and `%convert kmh mph 100`; verify the replies match the spec
