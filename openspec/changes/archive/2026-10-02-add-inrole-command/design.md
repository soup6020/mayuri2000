# Design

## Context

The bot connects with only the Guilds, Guild Messages, Direct Messages and Message Content intents. Its member cache therefore holds only members it has incidentally seen (message authors, interaction users, REST lookups), so it cannot answer "who has this role" from cache. `/roleinfo` already resolves roles (mention, ID, name) through the `cmd.Role` option type and gets counts from Discord's role member counts endpoint, which returns counts but not members.

Discord's "List Guild Members" REST endpoint returns up to 1000 members per request, paged by user ID, and requires the Server Members privileged intent to be *enabled for the application* in the Developer Portal. It does not require declaring the intent on the gateway connection.

## Goals / Non-Goals

**Goals:**
- Accurate role membership on demand without changing how the bot connects or what it caches.
- A clear message when the Server Members intent is disabled.

**Non-Goals:**
- Keeping a full member cache, presence data, or online/offline filters.
- Filtering by multiple roles, or exporting the list as a file.

## Decisions

**Fetch members over REST on demand rather than caching them through the gateway.**
Requesting the Guild Members gateway intent plus member chunking would keep every member in memory and add a startup member sync for each server, all for one occasional command. Paging the REST endpoint costs one request per 1000 members per invocation, which is negligible for this bot's small servers and needs no gateway or cache changes. If more member-dependent features arrive later, switching to a gateway member cache can be revisited then.

**Scan cap of 25,000 members (25 requests).**
This bounds latency and rate-limit use on a large server while staying far above the size of the servers the bot serves. If the cap is hit, the reply says the list may be incomplete (see spec).

**Run as a heavy command.**
Paging can take more than Discord's 3 second acknowledgement window on larger servers, so the command defers immediately like the image commands. Per-server serialisation is acceptable for an infrequent command.

**Detect the disabled intent from the API error.**
When the intent is disabled, the endpoint fails with a Missing Access style error (Discord JSON error 50001, HTTP 403). The command maps that to the spec's "enable the Server Members intent" message. Any other error goes through the normal error path.

**Presentation.**
Sort by the member's display name (server nickname, then global name, then username), case-insensitively. Each line is `<@id>` followed by the username in code formatting. Embed-based pages use the existing paginator (20 per page, owner-only buttons, 15 minute expiry). Mentions inside embeds never ping, and the reply's allowed mentions stay disabled. Bots are included, since they can hold roles too.

**Placement.**
The command goes next to `/roleinfo` in `internal/modules/core/users.go` and reuses `cmd.Role` resolution, so `%inrole server booster` behaves like `%roleinfo server booster`.

## Risks / Trade-offs

- [The intent isn't enabled when the change is deployed] → The command explains what to enable. The README setup steps are updated, and the change summary tells the owner to toggle it.
- [Large server latency or rate limits] → The 25,000 member cap, plus the deferred response. disgo's REST client already honours rate-limit buckets.
- [Membership changes between pages of a scan] → Accepted. The list is a point-in-time snapshot, as in the Discord client.

## Migration Plan

1. Enable **Server Members Intent** for the application in the Discord Developer Portal (Bot → Privileged Gateway Intents). The bot does not need a restart for REST access.
2. Deploy the new image. Slash commands sync automatically at startup.
3. Rollback: deploy the previous image. The intent can stay enabled harmlessly.
