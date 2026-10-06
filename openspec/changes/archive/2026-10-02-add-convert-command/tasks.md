# Tasks

## 1. Units package

- [x] 1.1 Create `internal/units` with a unit table (kind, symbol, accepted names, to/from base functions), Celsius/Fahrenheit/Kelvin with kelvin as the temperature base, case-insensitive lookup by any accepted name including degree-sign forms, and `Convert` that rejects mixed kinds, non-finite values, magnitudes over 1e12 and results below absolute zero; verify with table-driven tests for -40 °F = -40 °C, 0 °C = 273.15 K, 212 °F = 100 °C, identity conversion and each rejection
- [x] 1.2 Add value formatting (at most 2 decimals, trailing zeros trimmed, `-0` shown as `0`) and the `**100 °F = 37.78 °C**` statement format with `°C`, `°F` and `K`; verify with unit tests including 37.777… → 37.78 and 100.000 → 100

## 2. Command

- [x] 2.1 Register `/convert` (aliases `conv`, `unit`; Tools category) in the core module with options declared in this order: `from` and `to` (String, required, autocompleted from the unit table) then `value` (Float, required); reply with the formatted statement and report unknown units with the supported list; verify with registry-level tests for `/convert from:F to:C value:100`, text `%convert c f 30` → **30 °C = 86 °F**, `%convert c f` producing the usage line `%convert <from> <to> <value>`, the unknown-unit and absolute-zero messages, and that `TestCommandsValid` passes
- [x] 2.2 Verify autocomplete: `from`/`to` suggestions are filtered by prefix across accepted names, list all units for empty input, and stay within 25 choices; verify with a unit test of the autocomplete function

## 3. Docs and checks

- [x] 3.1 Update README.md Tools list with `convert` and examples (`/convert from:F to:C value:100`, `%convert c f 30`); verify by reading the section
- [x] 3.2 Run `golangci-lint run ./...` (0 issues), `go test ./...` and `nix flake check`; verify all pass and the image builds
- [x] 3.3 Manually: in a test server run `/convert` with autocomplete, `%convert c f 30` and `%convert c f -300`; verify the replies match the spec
