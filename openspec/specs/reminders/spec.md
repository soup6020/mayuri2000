# reminders Specification

## Purpose
Personal reminders delivered by DM or in a channel after a delay or at a clock time, persisted so they survive restarts, plus the time parsing rules they use.

## Requirements

### Requirement: Setting a reminder
`/remind when message [channel]` (remindme, reminder) SHALL schedule a reminder between 10 seconds and 21 days ahead. Without `channel` it SHALL be delivered by DM; with `channel` it SHALL be posted there. The reply SHALL show the due time as a Discord timestamp and the reminder's ID. A user MAY have at most 25 pending reminders; messages are at most 1000 characters.

#### Scenario: DM reminder
- **WHEN** a user runs `/remind when:3h12m message:laundry`
- **THEN** the bot confirms it will DM them in 3 hours 12 minutes

#### Scenario: Too far ahead
- **WHEN** a user asks for a reminder in 22 days
- **THEN** the bot refuses, stating the 21 day maximum

### Requirement: Time formats
`when` SHALL accept delays (`3h12m`, `90m`, `2 hours and 5 minutes`, `in 45 minutes`, a bare number of minutes), clock times (`20:15`, `8:15pm`, `8 pm`, `noon`, `midnight`; the next occurrence), day words (`today`, `tomorrow`, weekday names, `next friday`, optionally followed by `at` and a clock time) and ISO dates with a time (`2026-10-01 20:15`). Times in the past MUST be rejected.

#### Scenario: Clock time already passed
- **WHEN** it is 14:00 and a user asks for `9am`
- **THEN** the reminder is set for 09:00 tomorrow

#### Scenario: Weekday
- **WHEN** it is Wednesday and a user asks for `friday 18:00`
- **THEN** the reminder is set for this Friday at 18:00

### Requirement: Text form
In text form, `%remind [#channel] <when> <message>` SHALL find where the time ends without quoting, so `%remind tomorrow 9am call mom` sets a reminder for 09:00 tomorrow with message "call mom".

#### Scenario: Multi-word time
- **WHEN** a user sends `%remind next friday at 6pm pub`
- **THEN** the reminder message is "pub"

### Requirement: Time zones
Clock times and dates SHALL be read in the user's saved time zone, or UTC if none is saved; when UTC was assumed for a clock time, the confirmation MUST say so and point to `/timezone`.

#### Scenario: No saved zone
- **WHEN** a user without a saved zone runs `/remind when:20:15 message:x`
- **THEN** the confirmation notes the time was read as UTC

### Requirement: Channel reminders
Channel reminders MUST only be allowed in servers, in text channels of the same server where both the user and the bot can view and send messages.

#### Scenario: Channel the user can't post in
- **WHEN** a user targets a channel where they lack Send Messages
- **THEN** the bot refuses to post reminders there for them

### Requirement: Delivery
Reminders SHALL be delivered at their due time, including ones that came due while the bot was offline (marked late when more than 2 minutes overdue). A delivered reminder MUST only ping its owner, whatever its text contains. If the user's DMs are closed, the reminder SHALL be posted in the channel where it was set, mentioning them.

#### Scenario: Message contains @everyone
- **WHEN** a channel reminder's text includes `@everyone`
- **THEN** only the reminder's owner is pinged

### Requirement: Delivery failures
Transient delivery failures SHALL be retried with increasing delays up to 5 attempts. Permanent failures (deleted channel, missing access, DMs closed with no fallback) SHALL drop the reminder.

#### Scenario: Deleted channel
- **WHEN** the target channel was deleted before the reminder fired
- **THEN** the reminder is dropped without retrying

### Requirement: Listing and cancelling
`/reminders list` SHALL page through the invoker's pending reminders with IDs, times, targets and text. `/reminders cancel id` (delete, remove) SHALL cancel one of the invoker's own reminders; users MUST NOT cancel other users' reminders.

#### Scenario: Someone else's reminder
- **WHEN** a user tries to cancel a reminder ID owned by someone else
- **THEN** the bot says they have no reminder with that ID
