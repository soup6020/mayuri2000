# Proposal

## Why

There is no way to see who holds a role: `/roleinfo` only reports a member count, and the Discord client makes browsing a role's members awkward. Moderators and regular members both want a quick list, e.g. to check who has a game-night ping role or which members still carry an old role.

## What Changes

- **Add** a server-only `/inrole role` command (text aliases `rolemembers`, `whohas`, `members`) in the Info category that lists the members holding a role, resolved by mention, ID or name exactly like `/roleinfo`.
- Results are paginated (20 members per page), sorted by display name, and show each member's mention and username plus the total count.
- Listing a role's members requires the full member list, which the bot does not keep. It will page through Discord's "list guild members" endpoint on demand. That endpoint requires the privileged **Server Members** intent to be enabled for the application in the Developer Portal. When it isn't, the command reports exactly that instead of failing obscurely.
- Document the new intent requirement in the README alongside the existing Message Content requirement, and record it in the deployment spec.
- No existing commands change.

Discord limits touched: one new slash command (well under 100), one role option, description under 100 characters, embed descriptions kept well under 4096 characters per page. No context menu slots are used.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `info-commands`: adds the `/inrole` requirement (new command; existing requirements unchanged).
- `deployment`: adds a requirement stating which privileged gateway intents the application needs (Message Content, and now Server Members). Also corrects the existing "Nix build" and "Quality gates" requirements, which predate `openspec` being added to the devShell and spec validation to `nix flake check`.

## Impact

- Code: new command in `internal/modules/core/users.go` (beside `/roleinfo`, reusing `cmd.Role` option resolution and the paginator); member listing through `Rest().GetMembers` with pagination.
- Discord application settings: the Server Members intent must be enabled in the Developer Portal (a one-time manual step). No gateway intent change is needed because members are fetched over REST on demand.
- Performance: one REST request per 1000 members each time the command runs; the servers this bot serves are small. A safety cap bounds very large servers.
- Docs: README (command list, Developer Portal setup), CLAUDE.md unaffected beyond the command existing.
- No new dependencies, no schema changes.
