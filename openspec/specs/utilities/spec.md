# utilities Specification

## Purpose
Small standalone tools and fun commands: base64 encoding and decoding, unit conversion, and random tips from frog.tips.

## Requirements

### Requirement: Base64 encoding
`/base64 encode` (b64; encode aliases enc, e) SHALL encode text, or an attached file of at most 1 MiB, using standard base64 with padding, or the URL-safe alphabet without padding when `urlsafe` is set.

#### Scenario: Encode text
- **WHEN** a user runs `%b64 encode hello world`
- **THEN** the reply is `aGVsbG8gd29ybGQ=` in a code block

### Requirement: Base64 decoding
`/base64 decode` (dec, d) SHALL accept standard or URL-safe base64, with or without padding, ignoring whitespace. Printable UTF-8 results SHALL be shown as text; binary results SHALL be attached as a file named `decoded.png`, `.jpeg`, `.gif` or `.webp` when recognised as that image type, else `decoded.bin`.

#### Scenario: Binary result
- **WHEN** the input decodes to PNG data
- **THEN** the bot attaches `decoded.png`

### Requirement: Safe output
Text results longer than about 1900 characters, or containing a code fence (three backticks), SHALL be attached as a text file instead of a message, so results can't break out of the code block. Results MUST NOT mention anyone.

#### Scenario: Code fence in output
- **WHEN** decoded text contains a code fence and `@everyone`
- **THEN** it is attached as a file and nobody is pinged

### Requirement: Frog tips
`/frogtip` (frog, frogtips) SHALL reply with a random tip from frog.tips exactly as provided, prefixed with 🐸. Tips SHALL be taken from a cached batch (about 50 per API request) and a new batch fetched only when it runs out.

#### Scenario: Frog tip
- **WHEN** a user runs `/frogtip`
- **THEN** the bot replies with "🐸 " followed by an all-caps frog tip

### Requirement: Unit conversion
`/convert from to value` (conv, unit) SHALL convert a number between units of the same kind: temperature (Celsius, Fahrenheit, Kelvin), distance (kilometres, miles, metres, feet) and speed (km/h, mph). The reply SHALL show the input and result with unit symbols (`°C`, `°F`, `K`, `km`, `mi`, `m`, `ft`, `km/h`, `mph`), rounded to at most 2 decimal places without trailing zeros, e.g. **100 °F = 37.78 °C**. Converting a unit to itself SHALL return the value unchanged.

#### Scenario: Fahrenheit to Celsius
- **WHEN** a user runs `/convert from:F to:C value:100`
- **THEN** the bot replies **100 °F = 37.78 °C**

#### Scenario: Celsius to Kelvin
- **WHEN** a user runs `%convert c k 0`
- **THEN** the bot replies **0 °C = 273.15 K**

#### Scenario: Negative values
- **WHEN** a user converts -40 °F to Celsius
- **THEN** the bot replies **-40 °F = -40 °C**

#### Scenario: Kilometres to miles
- **WHEN** a user runs `%convert km mi 10`
- **THEN** the bot replies **10 km = 6.21 mi**

#### Scenario: Miles to kilometres
- **WHEN** a user runs `/convert from:miles to:kilometres value:26.2`
- **THEN** the bot replies **26.2 mi = 42.16 km**

#### Scenario: Feet to metres
- **WHEN** a user runs `%convert ft m 100`
- **THEN** the bot replies **100 ft = 30.48 m**

#### Scenario: km/h to mph
- **WHEN** a user runs `%convert kmh mph 100`
- **THEN** the bot replies **100 km/h = 62.14 mph**

#### Scenario: mph to km/h
- **WHEN** a user runs `%convert mph km/h 60`
- **THEN** the bot replies **60 mph = 96.56 km/h**

### Requirement: Unit names
Units SHALL be accepted case-insensitively by symbol or name: `c`, `°c`, `℃`, `celsius`, `centigrade`; `f`, `°f`, `℉`, `fahrenheit`; `k`, `°k`, `kelvin`, `kelvins` (temperatures with or without a degree sign); `km`, `kilometre(s)`, `kilometer(s)`; `mi`, `mile(s)`; `m`, `metre(s)`, `meter(s)`; `ft`, `foot`, `feet`; `km/h`, `kmh`, `kph`, `kmph`; `mph`. Slash command `from` and `to` options SHALL autocomplete the supported units. Unknown units MUST be rejected with a list of supported units.

#### Scenario: Unknown unit
- **WHEN** a user converts 10 from `rankine` to `c`
- **THEN** the bot says `rankine` isn't a supported unit and lists every supported unit, including Kelvin, Metres, Feet and mph

#### Scenario: Alternative spellings
- **WHEN** a user sends `%convert KPH MPH 50` or `%convert kilometers mile 5` or `%convert meters feet 3`
- **THEN** the units are recognised as km/h → mph, km → mi and m → ft

### Requirement: Text form
In text form, `%convert` SHALL take its arguments in the same order as the slash command: `%convert <from> <to> <value>`, e.g. `%convert c f 30` or `%convert °F celsius -40`. A missing argument or a value that is not a number MUST produce the usage line `%convert <from> <to> <value>`.

#### Scenario: From, to, value
- **WHEN** a user sends `%convert c f 30`
- **THEN** the bot replies **30 °C = 86 °F**

#### Scenario: Missing value
- **WHEN** a user sends `%convert c f`
- **THEN** the bot replies with the usage line

### Requirement: Conversion validation
`/convert` MUST reject temperatures below absolute zero (0 K, -273.15 °C, -459.67 °F), values that are not finite numbers, values whose magnitude exceeds 1e12, and conversions between different kinds of unit (e.g. distance to temperature, or distance to speed), each with a clear message. Negative distances and speeds SHALL be converted with their sign kept.

#### Scenario: Below absolute zero
- **WHEN** a user converts -300 °C to Fahrenheit
- **THEN** the bot says the temperature is below absolute zero

#### Scenario: Different kinds
- **WHEN** a user sends `%convert km mph 10`
- **THEN** the bot says the units measure different things and can't be converted

#### Scenario: Negative distance
- **WHEN** a user sends `%convert mi km -1`
- **THEN** the bot replies **-1 mi = -1.61 km**
