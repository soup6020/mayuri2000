# utilities Specification

## Purpose
Small standalone tools and fun commands: base64 encoding and decoding, and random tips from frog.tips.

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
