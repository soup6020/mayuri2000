# command-framework Specification

## Purpose
Defines how users invoke the bot: every command works both as a Discord slash command and through a configurable text prefix, with shared rules for arguments, permissions, aliases, help, pagination, errors and usage statistics.

## Requirements

### Requirement: Dual invocation
Every command SHALL be available both as a slash command and as a text command invoked with the prefix or by mentioning the bot, with the same behaviour and results.

#### Scenario: Same command two ways
- **WHEN** a user runs `/ping` or sends `%ping`
- **THEN** the bot replies with its gateway latency in both cases

#### Scenario: Mention as prefix
- **WHEN** a user sends `@Mayuri2000 ping`
- **THEN** the bot treats it as the `ping` command

### Requirement: Configurable prefix
The default prefix SHALL come from configuration (`%` unless set). A server MAY override it with `/prefix`, which requires Manage Server to change; `reset` restores the default. Prefixes MUST be at most 10 characters and contain no whitespace.

#### Scenario: Server override
- **WHEN** a member with Manage Server runs `/prefix new:!`
- **THEN** `!ping` works in that server and `%ping` no longer does

#### Scenario: Viewing the prefix
- **WHEN** any member runs `/prefix` without a value
- **THEN** the bot shows the prefix in effect

### Requirement: Prefix argument parsing
Text commands SHALL map whitespace-separated tokens (with quoting) to options in declaration order. An optional option MAY decline a token so it falls through to the next option. A final "rest" option SHALL take the remaining text. Attachments on the message SHALL fill attachment options.

#### Scenario: Optional option skipped
- **WHEN** a user sends `%rotate 45` without an image URL
- **THEN** `45` is used as the degrees and the image is found automatically

#### Scenario: Missing required argument
- **WHEN** a required argument is missing from a text command
- **THEN** the bot replies with the argument name and the command's usage line

### Requirement: Command groups
Commands MAY group subcommands, invoked as `/group sub` or `%group sub`. The group's permission and server-only restrictions SHALL apply to every subcommand. Sending a group name without a valid subcommand SHALL list the subcommands.

#### Scenario: Unknown subcommand
- **WHEN** a user sends `%rss nonsense`
- **THEN** the bot lists the `rss` subcommands with their usage

### Requirement: Permissions and server-only commands
Commands that declare required permissions SHALL use them as the slash command's default member permissions and SHALL also check them at runtime for both invocation styles. Server-only commands MUST refuse to run in DMs.

#### Scenario: Missing permission
- **WHEN** a member without Ban Members sends `%ban @someone`
- **THEN** the bot refuses, naming the missing permission

### Requirement: Server aliases for text commands
Members with Manage Server SHALL be able to create per-server aliases for text commands with `/alias add|remove|list|show`. An alias MUST NOT shadow an existing command or point to another alias, and a server MAY have at most 200. Invoking an alias SHALL run its command with the alias's stored arguments followed by the invocation's own arguments. Aliases SHALL NOT create slash commands.

#### Scenario: Alias with arguments
- **WHEN** `%alias add mg magik 5` has been run and a user sends `%mg https://x/cat.png`
- **THEN** the bot runs `%magik 5 https://x/cat.png`

#### Scenario: Alias shadowing a command
- **WHEN** someone tries `%alias add ban magik`
- **THEN** the bot refuses because `ban` is already a command

### Requirement: Help
`/help` SHALL list all commands grouped by category in pages, showing the prefix in effect. `/help <command>` SHALL show a command's description, usage, aliases and options; it SHALL resolve subcommands (`/help rss add`), groups (`/help rss`) and server aliases.

#### Scenario: Help for an alias
- **WHEN** a user runs `/help mg` where `mg` is a server alias
- **THEN** the bot says which command line the alias runs

### Requirement: Paginated replies
Multi-result replies SHALL use previous/next buttons that only the invoking user can use. Page state SHALL expire after 15 minutes. Button custom IDs on a message MUST be unique, including when there are exactly two pages.

#### Scenario: Another user presses a page button
- **WHEN** someone other than the invoker presses a page button
- **THEN** they get an ephemeral message saying only the invoker can flip pages

### Requirement: Heavy commands
Commands marked heavy (image processing, slow network work) SHALL acknowledge immediately (deferred reply or typing indicator), run with a 3 minute limit, and be serialised per server with a global concurrency bound.

#### Scenario: Slow image job
- **WHEN** a user runs `/gmagik` on a large GIF
- **THEN** Discord shows the bot thinking until the result is uploaded, without the interaction timing out

### Requirement: Error reporting
Errors meant for users SHALL be shown to the invoker (ephemerally for slash commands). Any other error or panic SHALL be logged and the user SHALL see a generic failure message; a panic MUST NOT crash the bot. If the first reply to an interaction fails, the error reply MUST still be delivered as the initial response.

#### Scenario: Unexpected failure
- **WHEN** a command handler panics
- **THEN** the bot logs the panic with a stack trace and tells the user something went wrong

### Requirement: Autocomplete
Autocompleted string options SHALL suggest values while a slash command is typed, returning at most 25 choices of at most 100 characters each.

#### Scenario: Time zone suggestions
- **WHEN** a user types `new_y` in `/time`'s zone option
- **THEN** `America/New_York` is suggested first

### Requirement: Startup validation
At startup the bot SHALL validate all commands against Discord's limits (names, 1-100 character descriptions, option counts, at most 100 commands, at most 5 message and 5 user context menus) and refuse to start if any are violated.

#### Scenario: Invalid command definition
- **WHEN** a command has a description longer than 100 characters
- **THEN** the bot exits at startup with an error naming the command

### Requirement: Usage statistics
Each command run SHALL be counted per session (by invocation type, failures, image jobs) and all-time in the database, keyed by full command name (e.g. `rss add`).

#### Scenario: Counting a run
- **WHEN** a user runs `/rss list`
- **THEN** the session and all-time counts for `rss list` increase by one
