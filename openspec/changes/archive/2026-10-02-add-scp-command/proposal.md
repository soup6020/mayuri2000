# Proposal

## Why

The old Red bot had an `scp` command that members used to share SCP Foundation articles. It only built a link to the long-gone `scp-wiki.net` domain and stopped at SCP-5999, while the series now runs to SCP-9999. The new bot has no equivalent. A lookup that shows the article's name and rating makes shared articles easier to recognise than a bare link.

## What Changes

- **Add** `/scp number` in the Search category, e.g. `/scp 3000` or `%scp 3000`. It replies with an embed linking to the article on `scp-wiki.wikidot.com`, titled with the number and its name ("SCP-3000 — Anantashesha"), and shows the article's community rating.
- Numbers from 1 to 9999 are accepted and shown zero-padded to three digits (`/scp 49` → SCP-049), matching the wiki's page names.
- Articles that don't exist yet are reported as such, and wiki outages produce the usual "isn't responding" message.
- Out of scope, as in the old cog's extra commands: joke (-J), explained (-EX) and archived (-ARC) articles, and article excerpts or object classes, which the wiki formats too inconsistently to extract reliably.

Discord limits touched: one new slash command (well under 100), one integer option with min/max, a single embed well under size limits. No context menu slots are used.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `search`: adds requirements for the `/scp` command (article lookup, naming and rating, missing articles).

## Impact

- Code: `/scp` in the search module, with a parser for the article page's rating and for the series index pages' article names. Series index pages are cached so repeated lookups don't refetch them.
- External service: `scp-wiki.wikidot.com` (fixed host, no API key). Up to two page loads per lookup (article and series index), with the index usually served from cache.
- Tests: saved fixtures of an article page, a series index page and a missing page.
- Docs: README Search list.
- No new dependencies, no database changes, no Discord settings changes.
