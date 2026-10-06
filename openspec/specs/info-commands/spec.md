# info-commands Specification

## Purpose
Informational commands: the current time in any time zone and per-user time zone preferences, server, user and role information, and runtime bot statistics.

## Requirements

### Requirement: Current time
`/time [zone]` (clock, tz) SHALL show the time in an IANA time zone with 24 and 12 hour clocks, date, abbreviation and UTC offset, plus a Discord timestamp showing the viewer's local time. Without a zone it SHALL use the user's saved zone, else UTC. Zone names SHALL match case-insensitively with spaces as underscores; unknown zones MUST get up to 5 suggestions.

#### Scenario: Case-insensitive zone
- **WHEN** a user runs `%time asia/kolkata`
- **THEN** the reply shows `Asia/Kolkata` at UTC+05:30

#### Scenario: Unknown zone
- **WHEN** a user runs `/time zone:london`
- **THEN** the bot suggests `Europe/London`

### Requirement: Time zone preference
`/timezone [zone]` (settimezone, mytz) SHALL show, set or (with `reset`) clear the user's saved IANA zone, replying ephemerally. The saved zone SHALL be used by `/time` and `/remind`. Zone options SHALL autocomplete from the full IANA database, including legacy names like `US/Eastern`.

#### Scenario: Set a zone
- **WHEN** a user runs `/timezone zone:europe/london`
- **THEN** the zone is saved as `Europe/London` and the current time there is shown

### Requirement: Server info
`/serverinfo` (si, guildinfo) SHALL be server-only and show owner, creation date, member and online counts, channel counts by kind, roles, emoji (animated) and stickers, boost level and count, verification level, locale, description, vanity URL, notable features, icon and banner.

#### Scenario: Server info
- **WHEN** a user runs `/serverinfo`
- **THEN** the bot shows an embed with the server's details and icon

### Requirement: User info
`/userinfo [user]` (ui, whois, user) and the "User Info" user context menu SHALL show a user's names, ID, account creation date, badges, avatar, banner and accent colour. For server members it SHALL also show join date, nickname, boosting date, active timeout, owner status, roles highest first (truncated with a count) and notable permissions, preferring server avatars, banners and role colour. Non-members SHALL be marked as such.

#### Scenario: Non-member
- **WHEN** a moderator runs `/userinfo` on a banned user's ID
- **THEN** the bot shows their account details and says they are not a member

### Requirement: Role info
`/roleinfo role` (ri, role) SHALL be server-only and resolve a role mention, ID or name (exact, then unique prefix, then unique substring; ambiguous names list candidates). It SHALL show mention, member count (from Discord's role member counts), creation date, colour, position, hoisted, mentionable, what manages it, notable permissions and icon.

#### Scenario: Role by partial name
- **WHEN** a user runs `%roleinfo booster`
- **THEN** the bot shows the Server Booster role

### Requirement: Bot statistics
`/stats` (botstats, uptime) SHALL show uptime, server count, gateway latency, commands run this session (slash, prefix, menu, failed, image jobs), all-time command count, memory use, and the top 5 commands for the session and all time.

#### Scenario: After restart
- **WHEN** the bot restarts
- **THEN** session counts reset while all-time counts are kept

### Requirement: Ping
`/ping` SHALL reply with the gateway latency.

#### Scenario: Ping
- **WHEN** a user runs `/ping`
- **THEN** the bot replies "Pong!" with the latency

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
