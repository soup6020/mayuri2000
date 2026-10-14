# safety Specification

## Purpose
Cross-cutting safety rules that every feature must follow: fetching user-supplied URLs, passing untrusted data to ImageMagick and ffmpeg, sandboxing those decoders, keeping secrets out of child processes, and preventing unwanted mentions in bot output.

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
Untrusted image data MUST be passed to ImageMagick with an explicit coder chosen from sniffed magic bytes (PNG, JPEG, GIF, WebP), never letting ImageMagick pick a coder, so data can't select MVG, MSL, SVG or similar. User text passed to ImageMagick MUST be escaped so a leading `@` can't read files and `%` can't expand properties. ImageMagick SHALL run in the decoder sandbox with memory, disk, area and time limits, under a policy allowing only the formats the bot uses.

#### Scenario: Caption reading a file
- **WHEN** a user captions an image with `@/etc/passwd`
- **THEN** the image shows the literal text `@/etc/passwd`

#### Scenario: Unused coder
- **WHEN** ImageMagick is asked to read an SVG, MVG or URL input
- **THEN** the security policy refuses it

### Requirement: Mention suppression
Bot output built from user or third-party content MUST NOT ping anyone. Replies SHALL disable mention parsing by default. Feed posts SHALL allow no mentions, and reminders SHALL allow only their owner.

#### Scenario: Feed item with @everyone
- **WHEN** an RSS item's title contains `@everyone`
- **THEN** the post is sent without pinging anyone

### Requirement: Secrets
The bot token and API keys SHALL be read from environment variables or `<NAME>_FILE` files and MUST NOT be logged, committed or included in images. Child processes (ImageMagick, ffmpeg) MUST receive only a fixed minimal environment, never the bot's, and MUST NOT be able to read secret files. The old Red data directory (`original/`) contains a live token and MUST stay out of git.

#### Scenario: Token from a secret file
- **WHEN** `DISCORD_TOKEN_FILE` points at a podman secret
- **THEN** the bot reads the token from that file

#### Scenario: Token in the environment
- **WHEN** the bot was started with `DISCORD_TOKEN` in its environment and runs ImageMagick
- **THEN** the ImageMagick process's environment contains no `DISCORD_TOKEN`, `GENIUS_TOKEN` or `TENOR_API_KEY`

### Requirement: ffmpeg input handling
Untrusted video MUST be passed to ffmpeg only as a local temporary file, with the MP4 demuxer forced and only local file and pipe protocols allowed, so input can't select other demuxers or make ffmpeg open network or other files. ffmpeg SHALL run in the decoder sandbox without reading stdin and be stopped when the command's time limit expires, and decoding SHALL be bounded by the video limits (10 seconds, 15 fps, 512 px).

#### Scenario: Playlist pointing elsewhere
- **WHEN** a user supplies a file that is really an HLS playlist referencing other URLs
- **THEN** it is rejected as not an MP4 and ffmpeg never fetches anything

#### Scenario: Hanging decode
- **WHEN** ffmpeg takes longer than the command's time limit
- **THEN** it is killed and the user is told processing took too long

### Requirement: Decoder sandbox
Every ImageMagick and ffmpeg process SHALL start inside a sandbox that allows reading only the program files and the system entries the decoders need, allows writing only to that job's own temporary directory, and denies network sockets, tracing other processes, creating namespaces, mounting, loading kernel modules, BPF, io_uring, userfaultfd, perf events and keyrings. The sandbox SHALL also cap CPU time, output file size and open files, and disable core dumps.

#### Scenario: Exploit reads the database
- **WHEN** a sandboxed decoder process tries to open the bot's database or a token file
- **THEN** the open fails with a permission error

#### Scenario: Exploit phones home
- **WHEN** a sandboxed decoder process tries to create a network socket
- **THEN** the call fails and no connection is made

#### Scenario: Another job's files
- **WHEN** two image commands run at the same time
- **THEN** neither decoder can read or write the other's temporary directory

### Requirement: Sandbox availability
If the decoder sandbox cannot be applied at startup, the bot SHALL disable the commands that need ImageMagick or ffmpeg and log a warning naming the missing feature, unless `DECODER_SANDBOX=best-effort` is set, in which case it SHALL apply every part that works and log which parts are missing. Other commands SHALL keep working.

#### Scenario: Kernel without Landlock
- **WHEN** the bot starts on a kernel without Landlock and `DECODER_SANDBOX` is unset
- **THEN** it logs that the decoder sandbox is unavailable, image and video effects that need ImageMagick or ffmpeg are disabled, and the bot still comes online
