# rss-feeds Specification

## Purpose
Channel subscriptions to RSS, Atom and JSON feeds: the bot periodically checks each feed and posts new items to the subscribed channel, replacing the old Red RSS cog.

## Requirements

### Requirement: Managing subscriptions
The `/rss` group (feed, feeds) SHALL be server-only and require Manage Channels. It SHALL provide `add <name> <url> [channel]`, `remove` (delete, del), `list [channel]`, `template`, `embed`, `force` and `find`. Names MUST be lowercase letters, digits, `-` or `_` (at most 32) and unique per channel; a server MAY have at most 50 feeds.

#### Scenario: Duplicate name
- **WHEN** a feed named `news` already exists in #general and someone adds another `news` there
- **THEN** the bot refuses

### Requirement: Feed name resolution
Commands taking a feed name SHALL use the `channel` option if given, otherwise the current channel, otherwise the only feed with that name in the server; ambiguous names MUST ask for a channel.

#### Scenario: Unique name elsewhere
- **WHEN** a user runs `/rss force name:jerj` in a different channel from the only `jerj` feed
- **THEN** the latest item is posted in the feed's own channel

### Requirement: No backlog on subscribe
Adding a feed SHALL fetch it immediately, reject URLs that aren't valid feeds, and mark every current item as already posted, so only items published afterwards are posted. The bot MUST check it can post in the target channel.

#### Scenario: Subscribe to a busy feed
- **WHEN** a user subscribes to a feed with 50 existing items
- **THEN** nothing is posted until a new item appears

### Requirement: Polling and posting
Feeds SHALL be checked every `RSS_INTERVAL` (default 5m, minimum 1m), fetching each distinct URL once per check with conditional requests. New items SHALL be posted oldest first, at most 5 per check (keeping the newest). Feed posts MUST NOT mention anyone.

#### Scenario: Several new items
- **WHEN** three new items appear between checks
- **THEN** they are posted in publication order in the next check

### Requirement: Delivery failures
An item that fails to post SHALL stay unposted and be retried on the next check (the conditional request cache is dropped so the feed is refetched). A subscription whose channel was deleted SHALL be removed. Fetch and post errors SHALL be shown in `/rss list`.

#### Scenario: Transient Discord error
- **WHEN** posting an item fails with a server error
- **THEN** the item is posted on a later check and not lost

### Requirement: Templates and embeds
Text posts SHALL use a per-feed template (default `$title\n$link`, typed `\n` means newline) with `$title`, `$link`, `$summary`, `$author`, `$published`, `$categories`, `$image`, `$feed` and `$name`; Red variables such as `$summary_plaintext` and `$content_image01` SHALL keep working. `/rss embed enabled:true` SHALL post rich embeds instead.

#### Scenario: Red template
- **WHEN** a feed uses the Red template `$title\n$link`
- **THEN** posts are the item title and link on separate lines

### Requirement: Manual posting and discovery
`/rss force` SHALL post a feed's latest item now and mark the feed's items as seen. `/rss find <website>` SHALL list the feeds a page advertises via `<link rel="alternate">`, or the URL itself if it is a feed.

#### Scenario: Find feeds
- **WHEN** a user runs `/rss find url:example.com`
- **THEN** the bot lists the RSS/Atom feed URLs the page links to

### Requirement: Feed fetching safety
Feed URLs SHALL be fetched through the safe HTTP client, limited to 5 MiB, and MUST NOT reach loopback, private or other non-public addresses.

#### Scenario: Internal URL
- **WHEN** someone adds a feed at `http://192.168.1.1/feed.xml`
- **THEN** the bot refuses to fetch it
