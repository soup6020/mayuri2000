# Proposal

## Why

Members regularly ask for quick conversions in chat, temperature most often (sharing weather, cooking, game settings). A `/convert` command answers them without leaving Discord. It starts with temperature and is built so more units can be added later.

## What Changes

- **Add** `/convert from to value` (text aliases `conv`, `unit`) in the Tools category. It converts a number between Celsius, Fahrenheit and Kelvin, e.g. `/convert from:F to:C value:100` → **100 °F = 37.78 °C**.
- The `from` and `to` options autocomplete the supported units. Units can be typed by name or symbol, case-insensitively (`c`, `°C`, `celsius`, `f`, `fahrenheit`, `k`, `kelvin`, …).
- The text form takes the same order as the slash options: `%convert c f 30` (from, to, value).
- Temperatures below absolute zero, unknown units, and conversions between different kinds of unit are rejected with a clear message.
- No existing command changes behaviour.

Discord limits touched: one new slash command (well under 100), three options with descriptions under 100 characters, unit autocomplete within 25 choices. No context menu slots are used.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `utilities`: adds requirements for the `/convert` command (unit parsing, temperature conversion and validation, output format).

## Impact

- Code: a new unit conversion package (unit table by kind: temperature now, more later) and a `/convert` command in the core module. No framework changes.
- Docs: README Tools list.
- No new dependencies, no database changes, no Discord settings changes.
