# Spec Delta

## ADDED Requirements

### Requirement: Video input and output
Image effects SHALL accept MP4 video. The first 10 seconds SHALL be used, at most 15 frames per second, scaled so the longest side is at most 512 px. An MP4 input SHALL produce an MP4 output with the effect applied to every frame and the source's audio track kept unchanged. Tenor and Klipy GIFs (read as MP4) SHALL produce a GIF output without audio. Effects that use a single frame SHALL use the video's first frame.

#### Scenario: MP4 in, MP4 out
- **WHEN** a user runs `/gmagik` on an MP4 attachment with sound
- **THEN** the bot replies with an MP4 where every frame is magik'd and the original audio plays

#### Scenario: Tenor or Klipy GIF
- **WHEN** a user runs `/gmagik` right after someone posts a Klipy GIF
- **THEN** the bot reads the GIF's MP4 and replies with an animated GIF that loops like the original

#### Scenario: Long video
- **WHEN** a user runs `/invert` on a 40 second MP4
- **THEN** the bot processes only the first 10 seconds and returns a 10 second MP4

#### Scenario: Still-only effect on video
- **WHEN** a user runs `/jpeg` on an MP4
- **THEN** the bot applies it to the first frame and replies with a JPEG

### Requirement: Video availability
Video input requires ffmpeg. When ffmpeg is unavailable, video inputs MUST be refused with a message saying video support needs ffmpeg, and image inputs SHALL keep working.

#### Scenario: No ffmpeg
- **WHEN** ffmpeg is not installed and a user runs `/invert` on an MP4
- **THEN** the bot says video support needs ffmpeg

## MODIFIED Requirements

### Requirement: Finding the input image
Image commands SHALL use the first image or MP4 video found in this order: the `image` attachment option; the `url` option or leftover text arguments (image and MP4 links, Tenor links, custom emoji, user mentions or IDs as avatars); the `user` option's avatar; other attachments on the invoking message; the replied-to or context-menu target message; then the last 10 messages in the channel. Messages SHALL be searched for image and MP4 attachments, image embeds, and `gifv` or video embeds (as posted from the Tenor and Klipy pickers). When the command's own message contains a Tenor or Klipy link, the bot SHALL use the embed Discord attaches to it, waiting briefly for it to appear. If none is found the bot MUST say how to provide one.

#### Scenario: Image from history
- **WHEN** a user runs `/invert` with no arguments right after someone posted a picture
- **THEN** the bot inverts that picture

#### Scenario: Avatar by mention
- **WHEN** a user sends `%triggered @friend`
- **THEN** the bot uses @friend's avatar (animated avatars as GIF)

#### Scenario: Tenor link
- **WHEN** a user sends `%magik https://tenor.com/view/...`
- **THEN** the bot resolves the Tenor page to its MP4 rendition (via the Tenor API when a key is configured, otherwise by scraping) and treats it as a Tenor GIF

#### Scenario: Klipy embed in history
- **WHEN** a user runs `/deepfry` with no arguments right after someone posted a Klipy GIF from the picker
- **THEN** the bot uses the MP4 from that message's embed

### Requirement: Input limits and formats
Inputs SHALL be PNG, JPEG, GIF, WebP or MP4 identified by content, at most 25 MiB. Images SHALL be at most 40 megapixels per frame and animated images SHALL keep at most 200 frames; videos SHALL be limited as described in "Video input and output". Other video containers (MOV, WebM and so on), unsupported or oversized inputs MUST produce a clear user-facing error.

#### Scenario: Not an image
- **WHEN** the URL points at an HTML page
- **THEN** the bot replies that it doesn't look like a supported image or MP4 video

#### Scenario: WebM video
- **WHEN** a user runs `/invert` on a WebM file
- **THEN** the bot replies that only MP4 videos are supported

### Requirement: Animated images
Effects SHALL process every frame of an animated GIF and return a GIF, and every frame of a video as described in "Video input and output", unless the effect's contract says otherwise. Random effects SHALL use the same randomness on every frame so animations don't flicker.

#### Scenario: GIF in, GIF out
- **WHEN** a user runs `/invert` on an animated GIF
- **THEN** the result is an animated GIF with every frame inverted
