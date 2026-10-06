# search Specification

## Purpose
Lookup commands against external services: Urban Dictionary, Wikipedia (search, featured content and random articles), YouTube, Genius lyrics, and NixOS packages and options from search.nixos.org.

## Requirements

### Requirement: Service failures
Every search command SHALL tell the user when the external service is unavailable or returns nothing, rather than failing silently or with a generic error.

#### Scenario: Service down
- **WHEN** Urban Dictionary times out
- **THEN** the bot replies that Urban Dictionary isn't responding right now

### Requirement: Urban Dictionary
`/urban query` (ud, urbandictionary) SHALL show every definition as a page, sorted by net votes, with the example, vote counts, author and date. Bracketed `[terms]` SHALL become links, falling back to plain text if links would exceed embed limits.

#### Scenario: Definition pages
- **WHEN** a user runs `/urban yeet`
- **THEN** the bot shows paginated definitions, highest net votes first

### Requirement: Wikipedia search
`/wikipedia query` (wiki) SHALL search English Wikipedia article titles, skip disambiguation pages, and show each result's intro (cut to about 1000 characters with a read-more link), main image and last-edited time as a page.

#### Scenario: Disambiguation skipped
- **WHEN** a user searches `mercury`
- **THEN** the disambiguation page is not among the results

### Requirement: Wikipedia featured content
`/wikifeatured [date]` (featured, tfa, potd) SHALL show pages for the featured article, picture of the day (with artist and license) and the previous day's 10 most read articles with view counts. `date` accepts `YYYY-MM-DD`, `today` or `yesterday`, uses UTC days, and MUST be between 2016-01-01 and today.

#### Scenario: Future date
- **WHEN** a user asks for tomorrow's date
- **THEN** the bot says the date is in the future

### Requirement: Random Wikipedia article
`/wikirandom` (randomwiki, rwiki) SHALL show a random non-disambiguation article with a reroll button. Only the invoker SHALL be able to reroll; others MUST be told to run the command themselves.

#### Scenario: Reroll
- **WHEN** the invoker presses 🎲 Another
- **THEN** the message is replaced with a different random article

### Requirement: YouTube
`/youtube query` (yt) SHALL post the top video URL and `/ytsearch query` SHALL page through up to 25 video URLs, found by scraping the YouTube results page (no API key).

#### Scenario: Top result
- **WHEN** a user runs `/youtube never gonna give you up`
- **THEN** the bot posts a youtube.com/watch URL

### Requirement: Genius lyrics
`/lyrics query` (genius) SHALL find the best Genius match and show its lyrics in pages with section headers kept and page chrome removed. Song search SHALL use the official API when `GENIUS_TOKEN` is set, falling back to Genius's public search.

#### Scenario: Lyrics found
- **WHEN** a user runs `/lyrics never gonna give you up`
- **THEN** the bot shows the lyrics with headers like `[Chorus]`

### Requirement: NixOS package search
`/nixpkgs query [channel]` (nix, nixos, nixsearch) SHALL search search.nixos.org packages on the latest stable channel or unstable, ranking exact attribute/pname matches first, and show up to 10 results with version, description, a `nix shell nixpkgs#<attr>` snippet, homepage, license, platforms, programs, maintainers and a nixpkgs source link.

#### Scenario: Exact match first
- **WHEN** a user searches `firefox`
- **THEN** `firefox` is shown before `firefox-unwrapped`

### Requirement: NixOS option search
`/nixoption query [channel]` (nixopt, nixopts, nixoptions, option) SHALL show up to 10 NixOS options with description converted from HTML to Discord markdown, type, default and example as Nix code blocks, and a link to where the option is declared.

#### Scenario: Option details
- **WHEN** a user searches `services.openssh.enable`
- **THEN** the result shows type `boolean` and default `false`

### Requirement: search.nixos.org backend discovery
The bot SHALL read the search backend's schema version, public credentials and channel list from search.nixos.org's frontend bundle, cache them for 6 hours, and fall back to built-in defaults if that fails, so site updates don't break the commands.

#### Scenario: Frontend unreachable
- **WHEN** the frontend bundle can't be fetched
- **THEN** searches still work using the built-in defaults

### Requirement: SCP articles
`/scp number` SHALL accept numbers from 1 to 9999 and reply with an embed linking to `https://scp-wiki.wikidot.com/scp-NNN` (the number zero-padded to at least three digits), titled "SCP-NNN — Name" using the article's name from the wiki's series index (just "SCP-NNN" when the index has no name for it), and showing the article's rating. Text form is `%scp <number>`.

#### Scenario: Article with a name
- **WHEN** a user runs `/scp 3000`
- **THEN** the bot replies with an embed titled "SCP-3000 — Anantashesha" linking to `https://scp-wiki.wikidot.com/scp-3000`, with the article's rating

#### Scenario: Zero padding
- **WHEN** a user sends `%scp 49`
- **THEN** the embed is titled with SCP-049 and links to `https://scp-wiki.wikidot.com/scp-049`

#### Scenario: Out of range
- **WHEN** a user sends `%scp 12345`
- **THEN** the bot rejects the number, saying it must be between 1 and 9999

### Requirement: Missing SCP articles
When the wiki has no page for the number, `/scp` MUST say that SCP-NNN doesn't exist yet instead of linking to a missing page. When the wiki can't be reached, it SHALL say the SCP wiki isn't responding.

#### Scenario: Unwritten article
- **WHEN** a user asks for a number whose page returns "not found"
- **THEN** the bot says that SCP doesn't exist yet
