# Spec Delta

## ADDED Requirements

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
