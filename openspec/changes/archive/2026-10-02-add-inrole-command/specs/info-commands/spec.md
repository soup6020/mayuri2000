# Spec Delta

## ADDED Requirements

### Requirement: Role members
`/inrole role` (rolemembers, whohas, members) SHALL be server-only, resolve the role exactly like `/roleinfo`, and list every member holding it, sorted by display name, 20 per page, each with mention and username, plus the total count. Listing MUST NOT ping anyone. An empty role SHALL be reported as having no members.

#### Scenario: Role with members
- **WHEN** a user runs `%inrole game night` and five members have the role
- **THEN** the bot shows one page listing those five members and a total of 5, without pinging them

#### Scenario: Many members
- **WHEN** a role has 45 members
- **THEN** the bot shows three pages of at most 20 members, navigable with the page buttons

#### Scenario: Empty role
- **WHEN** a user runs `/inrole` on a role nobody has
- **THEN** the bot says the role has no members

### Requirement: Role members availability
Listing role members requires the Server Members privileged intent. When Discord refuses to list members because it is not enabled, `/inrole` MUST tell the user that the bot owner needs to enable the Server Members intent, instead of reporting a generic error. For servers larger than 25,000 members the list SHALL cover the first 25,000 scanned and say it may be incomplete.

#### Scenario: Intent disabled
- **WHEN** the Server Members intent is disabled in the Developer Portal and a user runs `/inrole`
- **THEN** the bot replies that the Server Members intent must be enabled for this command
