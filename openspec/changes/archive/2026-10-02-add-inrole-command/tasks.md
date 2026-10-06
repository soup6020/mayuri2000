# Tasks

## 1. Member listing

- [x] 1.1 Add a role-member scanner beside `/roleinfo` in `internal/modules/core/users.go` that pages Discord's list-members endpoint 1000 at a time through a small interface (satisfied by the REST client), keeps members holding the role, and stops at 25,000 scanned, reporting whether it truncated; verify with a unit test using a fake lister that covers multiple pages, the cap and an empty role
- [x] 1.2 Map Discord's Missing Access (50001) error from member listing to a user error telling the owner to enable the Server Members intent; verify with a unit test that the fake lister's 50001 error produces that message and other errors pass through unchanged

## 2. Command

- [x] 2.1 Add a pure page builder that sorts members by display name (server nickname, then global name, then username, case-insensitively), renders `<@id>` plus the username 20 per page with the total count, and adds an "may be incomplete" note when truncated; verify with unit tests for ordering, the 45 members → 3 pages case and embed descriptions under 4096 characters
- [x] 2.2 Register `/inrole role` (aliases `rolemembers`, `whohas`, `members`; Info category; server-only; heavy) using the `cmd.Role` option with `Rest: true`, replying "no members" for empty roles and paginating otherwise with mentions disabled; verify `TestCommandsValid` passes and `/help inrole` shows the usage

## 3. Docs and checks

- [x] 3.1 Update README.md: add `inrole` to the Info commands and add the Server Members intent to the Developer Portal setup next to Message Content; verify by reading the rendered sections
- [x] 3.2 Run `golangci-lint run ./...` (0 issues), `go test ./...` and `nix flake check` (tests, lint and spec validation); verify all pass
- [x] 3.3 Manually: enable the Server Members intent, run `/inrole` and `%inrole <name>` in a test server on a role with members and on an empty role; verify the listings, page buttons and that nobody is pinged
