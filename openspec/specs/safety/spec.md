# safety Specification

## Purpose
Cross-cutting safety rules that every feature must follow: fetching user-supplied URLs, passing untrusted data to ImageMagick, and preventing unwanted mentions in bot output.

## Requirements

### Requirement: User-supplied URLs
Any URL that comes from a user (image URLs, feed URLs, watermarks, attachments) SHALL be fetched only through the safe HTTP client, which MUST allow only http and https, reject URLs with credentials, and refuse to connect to loopback, private, link-local, carrier-grade NAT, multicast, unspecified, reserved and NAT64 addresses. The check MUST apply to the resolved IP at connection time, covering redirects and DNS rebinding.

#### Scenario: Internal address by name
- **WHEN** a user gives an image URL whose hostname resolves to 127.0.0.1
- **THEN** the download is refused

#### Scenario: Redirect to an internal address
- **WHEN** a public URL redirects to http://169.254.169.254/
- **THEN** the redirect target is refused

### Requirement: ImageMagick input handling
Untrusted image data MUST be passed to ImageMagick with an explicit coder chosen from sniffed magic bytes (PNG, JPEG, GIF, WebP), never letting ImageMagick pick a coder, so data can't select MVG, MSL, SVG or similar. User text passed to ImageMagick MUST be escaped so a leading `@` can't read files and `%` can't expand properties. ImageMagick SHALL run with memory, disk, area and time limits.

#### Scenario: Caption reading a file
- **WHEN** a user captions an image with `@/etc/passwd`
- **THEN** the image shows the literal text `@/etc/passwd`

### Requirement: Mention suppression
Bot output built from user or third-party content MUST NOT ping anyone. Replies SHALL disable mention parsing by default. Feed posts SHALL allow no mentions, and reminders SHALL allow only their owner.

#### Scenario: Feed item with @everyone
- **WHEN** an RSS item's title contains `@everyone`
- **THEN** the post is sent without pinging anyone

### Requirement: Secrets
The bot token and API keys SHALL be read from environment variables or `<NAME>_FILE` files and MUST NOT be logged, committed or included in images. The old Red data directory (`original/`) contains a live token and MUST stay out of git.

#### Scenario: Token from a secret file
- **WHEN** `DISCORD_TOKEN_FILE` points at a podman secret
- **THEN** the bot reads the token from that file

### Requirement: ffmpeg input handling
Untrusted video MUST be passed to ffmpeg only as a local temporary file, with the MP4 demuxer forced and only local file and pipe protocols allowed, so input can't select other demuxers or make ffmpeg open network or other files. ffmpeg SHALL run without reading stdin and be stopped when the command's time limit expires, and decoding SHALL be bounded by the video limits (10 seconds, 15 fps, 512 px).

#### Scenario: Playlist pointing elsewhere
- **WHEN** a user supplies a file that is really an HLS playlist referencing other URLs
- **THEN** it is rejected as not an MP4 and ffmpeg never fetches anything

#### Scenario: Hanging decode
- **WHEN** ffmpeg takes longer than the command's time limit
- **THEN** it is killed and the user is told processing took too long
