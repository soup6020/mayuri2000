## MODIFIED Requirements

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
