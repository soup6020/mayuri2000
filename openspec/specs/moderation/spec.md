# moderation Specification

## Purpose
Server moderation commands: kick, ban (including hackban by user ID for people not in the server), unban, timeout and message purging, with role hierarchy checks and audit log reasons.

## Requirements

### Requirement: Permissions and audit log
Moderation commands SHALL be server-only and require the matching Discord permission: Kick Members (kick), Ban Members (ban, hackban, unban), Moderate Members (timeout, untimeout), Manage Messages (purge). The bot MUST check it holds the permission itself and SHALL record the reason and the moderator in the audit log.

#### Scenario: Bot lacks permission
- **WHEN** a moderator runs `/ban` but the bot's role lacks Ban Members
- **THEN** the bot says it needs the Ban Members permission

### Requirement: Role hierarchy
Kick, ban, hackban (for members), timeout and untimeout MUST refuse to act on the invoker, the bot, or the server owner, and MUST refuse when the target's highest role is equal to or above the invoker's (unless the invoker owns the server) or the bot's.

#### Scenario: Target outranks moderator
- **WHEN** a moderator tries to kick someone with a higher role
- **THEN** the bot refuses and explains why

### Requirement: Kick and ban
`/kick user [reason]` SHALL remove a member. `/ban user [delete_days 0-7] [reason]` SHALL ban any user, whether or not they are in the server, deleting that many days of their messages. Members SHALL be sent a DM with the server name and reason before being kicked or banned, when possible.

#### Scenario: Ban a non-member
- **WHEN** a moderator bans a user ID that isn't in the server
- **THEN** the ban succeeds without a hierarchy check or DM

### Requirement: Hackban
`/hackban user_ids [delete_days] [reason]` SHALL ban up to 50 users by ID or mention, separated by spaces or commas, including users not in the server. It SHALL report each ID's outcome: banned, already banned, no such user, invalid ID, refused (hierarchy), or failed. In text form the arguments are `<ids...> [single-digit delete_days] [reason...]`.

#### Scenario: Mixed batch
- **WHEN** a moderator hackbans one valid ID, one already-banned ID and `abc`
- **THEN** the reply lists banned, already banned and "not a valid user ID" respectively, with a summary count

### Requirement: Unban
`/unban user_id [reason]` SHALL remove a ban by ID or mention and report when the user wasn't banned.

#### Scenario: Not banned
- **WHEN** a moderator unbans someone who isn't banned
- **THEN** the bot says that user isn't banned

### Requirement: Timeout
`/timeout user duration [reason]` SHALL time out a member for durations like `10m`, `1h30m`, `2d` or a bare number of minutes, between 1 minute and 28 days. Administrators MUST NOT be timed out. `/untimeout` SHALL remove a timeout. The confirmation SHALL show when the timeout ends.

#### Scenario: Too long
- **WHEN** a moderator runs `/timeout duration:30d`
- **THEN** the bot says Discord allows at most 28 days

### Requirement: Purge
`/purge count [user]` SHALL bulk-delete up to 100 recent messages newer than 14 days, optionally only from one user. In text form the invoking message SHALL also be deleted. The bot SHALL report how many messages were deleted.

#### Scenario: Old messages
- **WHEN** every recent message in the channel is older than 14 days
- **THEN** the bot says there is nothing it can purge
