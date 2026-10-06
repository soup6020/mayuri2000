# Design

## Context

The SCP wiki (`scp-wiki.wikidot.com`) has no public API. Observed behaviour, checked against the live site:

- An article page (`/scp-3000`) is about 115 KB of HTML. Its `<title>` is just "SCP-3000 - SCP Foundation" (no article name). Its rating appears in the page's rating module (`<span class="number …">+3029</span>`).
- Missing articles return **HTTP 404** (e.g. `/scp-12345`). Some numbers exist only as placeholder pages (SCP-9999).
- Article names live on the series index pages: `/scp-series` (001–999), then `/scp-series-2` … `/scp-series-10` (1000–9999). Entries look like `<li><a href="/scp-3000">SCP-3000</a> - Anantashesha</li>`. Some names contain inline formatting tags.
- Object class and images vary per article (custom banners, CSS-only layouts, site chrome images), so they can't be extracted reliably. They are left out (see proposal).

The search module already fetches fixed third-party sites with a browser-like user agent (Genius, YouTube) and tests its parsers against saved fixtures.

## Goals / Non-Goals

**Goals:**
- Name, rating and link for any SCP-001 to SCP-9999 page, in one quick reply.
- Robust to missing names (fall back to the bare number) and to the wiki being down.

**Non-Goals:**
- -J, -EX and -ARC articles, tales, excerpts, object classes, images.
- Non-English branches.

## Decisions

**Fetch the article page for existence and rating; take the name from the series index.**
The article page is the only reliable signal that the article exists (404 otherwise) and the only source of the rating. The name is not on the article page, so it comes from the series index. Alternative considered: posting only the link, as the old cog did. That's simpler, but it can't tell members whether the article exists and gives no name.

**Cache series index pages for 6 hours.**
Each index is about 100 KB, and names change rarely. An in-memory cache keyed by series page means a typical lookup costs one request (the article) after the first. If the index can't be fetched, the reply falls back to the bare "SCP-NNN" title rather than failing.

**Number handling.**
An Int option with min 1 and max 9999 lets Discord and the framework enforce the range for both invocation styles. Page names use `%03d` (scp-049, scp-3000). The series page is `scp-series` for numbers below 1000, else `scp-series-(n/1000+1)`.

**Parsing.**
Use the existing HTML tokenizer (`golang.org/x/net/html`) for the series index: find `<a href="/scp-NNN">` inside list items, then take the following text with tags stripped and the leading " - " removed. Use a regex for the rating number on the article page, which is a single, stable element. Fixtures of both pages pin the formats.

**Requests.**
Use the search module's existing fetch helper with a browser user agent and its 15 second timeout. The host is fixed (not user-supplied), so the safe HTTP client isn't required. A 404 maps to the "doesn't exist yet" user error. Other failures map to "the SCP wiki isn't responding".

**Presentation.**
The embed title is "SCP-3000 — Anantashesha", linking to the article. The description shows the rating (e.g. "Rating: **+3029**"), and the footer reads "SCP Foundation · CC BY-SA 3.0". There are no mentions, and the embed never includes page content.

## Risks / Trade-offs

- [Wikidot markup changes break name or rating parsing] → Parsers degrade gracefully (no name means the bare number, no rating means it's omitted). Saved fixtures show what changed when it happens.
- [Rate limiting or blocking of scraping] → One cached index plus one article request per lookup, at the low volume of a single bot.
- [Placeholder pages count as existing] → Accepted. They are real pages on the wiki, and the link shows the placeholder.
