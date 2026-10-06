# Tasks

## 1. Fixtures and parsers

- [x] 1.1 Save trimmed fixtures from the live wiki in `internal/modules/search/testdata/`: the SCP-3000 article page, the `scp-series-4` index page, and note the 404 behaviour for a missing number; verify the files exist and contain the rating element and the SCP-3000 index entry
- [x] 1.2 Add `ParseSCPRating(page)` (returns e.g. "+3029", or empty when absent) and `ParseSCPNames(index)` (number → name, tags stripped, leading " - " removed); verify with fixture tests for SCP-3000 → "Anantashesha" and "+3029", an entry with inline formatting, and missing data
- [x] 1.3 Add series page selection (`scp-series` below 1000, else `scp-series-(n/1000+1)`) and zero-padded page names; verify with unit tests for 1 → scp-001/scp-series, 49 → scp-049, 999 → scp-series, 1000 → scp-series-2, 3000 → scp-series-4, 9999 → scp-series-10

## 2. Command

- [x] 2.1 Add a series index cache (6 hours, per series page) and the lookup that fetches the article (404 → "SCP-NNN doesn't exist yet", other failures → "the SCP wiki isn't responding") and the name (falling back to the bare number if the index fails); verify with unit tests using a fake fetcher for the found, missing, index-failure and outage cases, and that the cache avoids a second index fetch
- [x] 2.2 Register `/scp number` (Search category, Int option min 1, max 9999) replying with the embed titled "SCP-NNN — Name", linking to the article, showing the rating, with the "SCP Foundation · CC BY-SA 3.0" footer; verify with registry-level tests for `/scp 3000`, `%scp 49` and `%scp 12345` (range rejection), and that `TestCommandsValid` passes

## 3. Docs and checks

- [x] 3.1 Update README.md Search list with `scp <number>`; verify by reading the section
- [x] 3.2 Run `golangci-lint run ./...` (0 issues), `go test ./...` and `nix flake check`; verify all pass and the image builds
- [x] 3.3 Live check against the wiki (a throwaway test run, not committed) for SCP-3000, SCP-049 and a missing number; then manually run `/scp 3000` and `%scp 49` in Discord and verify the embeds
