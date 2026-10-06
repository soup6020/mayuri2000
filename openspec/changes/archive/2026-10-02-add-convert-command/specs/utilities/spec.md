# Spec Delta

## ADDED Requirements

### Requirement: Unit conversion
`/convert from to value` (conv, unit) SHALL convert a number between temperature units: Celsius, Fahrenheit and Kelvin. The reply SHALL show the input and result with unit symbols (`°C`, `°F`, `K`), rounded to at most 2 decimal places without trailing zeros, e.g. **100 °F = 37.78 °C**. Converting a unit to itself SHALL return the value unchanged.

#### Scenario: Fahrenheit to Celsius
- **WHEN** a user runs `/convert from:F to:C value:100`
- **THEN** the bot replies **100 °F = 37.78 °C**

#### Scenario: Celsius to Kelvin
- **WHEN** a user runs `%convert c k 0`
- **THEN** the bot replies **0 °C = 273.15 K**

#### Scenario: Negative values
- **WHEN** a user converts -40 °F to Celsius
- **THEN** the bot replies **-40 °F = -40 °C**

### Requirement: Unit names
Units SHALL be accepted case-insensitively by symbol or name, with or without a degree sign: `c`, `°c`, `℃`, `celsius`, `centigrade`; `f`, `°f`, `℉`, `fahrenheit`; `k`, `°k`, `kelvin`, `kelvins`. Slash command `from` and `to` options SHALL autocomplete the supported units. Unknown units MUST be rejected with a list of supported units.

#### Scenario: Unknown unit
- **WHEN** a user converts 10 from `rankine` to `c`
- **THEN** the bot says `rankine` isn't a supported unit and lists Celsius, Fahrenheit and Kelvin

### Requirement: Text form
In text form, `%convert` SHALL take its arguments in the same order as the slash command: `%convert <from> <to> <value>`, e.g. `%convert c f 30` or `%convert °F celsius -40`. A missing argument or a value that is not a number MUST produce the usage line `%convert <from> <to> <value>`.

#### Scenario: From, to, value
- **WHEN** a user sends `%convert c f 30`
- **THEN** the bot replies **30 °C = 86 °F**

#### Scenario: Missing value
- **WHEN** a user sends `%convert c f`
- **THEN** the bot replies with the usage line

### Requirement: Conversion validation
`/convert` MUST reject temperatures below absolute zero (0 K, -273.15 °C, -459.67 °F), values that are not finite numbers, values whose magnitude exceeds 1e12, and conversions between different kinds of unit, each with a clear message.

#### Scenario: Below absolute zero
- **WHEN** a user converts -300 °C to Fahrenheit
- **THEN** the bot says the temperature is below absolute zero
