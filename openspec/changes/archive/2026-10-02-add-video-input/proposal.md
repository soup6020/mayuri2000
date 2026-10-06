# Proposal

## Why

Most "GIFs" posted in Discord now come from the Tenor and Klipy pickers, and those are really silent looping MP4s attached as `gifv` embeds. The image commands can't read Klipy at all (its pages block bots), and they read Tenor only through its lower-quality GIF rendition. Members also share short MP4 clips that they'd like to magik, deep fry or caption. Accepting MP4 as an input for every image effect fixes both.

## What Changes

- **MP4 becomes a general input** for the image effect commands (gmagik, magik, invert, deepfry, shake, caption, …), in addition to PNG/JPEG/GIF/WebP. Only MP4 is supported; MOV, WebM and other containers are out of scope and are rejected with a clear message.
- **MP4 in → MP4 out, with the original audio kept.** The effect is applied to every frame and the result is re-encoded as MP4 with the source's audio track copied unchanged.
- **Tenor and Klipy GIFs → GIF out.** They are read from their MP4 renditions for quality and size, but posted back as GIF so the result autoplays and loops like what people posted. Their audio, if any, is dropped.
- **Finding inputs** also picks up MP4 attachments, `gifv` and video embeds on messages (how Tenor and Klipy posts arrive), and direct `.mp4` links. Tenor links now resolve to the MP4 rendition. A Tenor or Klipy link in the command's own message uses the embed Discord attaches to it.
- **Limits** for video input: the first 10 seconds, at most 15 frames per second, scaled so the longest side is at most 512 px (so at most 150 frames). These are on top of the existing 25 MiB download limit and the channel's upload limit.
- **Still-only effects** (jpeg, glitch, vw, petimg, triggered, merge, rip and others that use a single frame) use the video's first frame, the same as they do for GIFs today.
- **ffmpeg** is added to the container image and dev shell and is configurable via `FFMPEG_PATH`. Without ffmpeg, image input keeps working and video input reports that ffmpeg is unavailable.

Discord limits touched: no new commands or options; outputs are still bounded by the channel's upload limit, as today.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `image-commands`: input discovery (video attachments, gifv/video embeds, MP4 links, Tenor MP4), accepted formats and limits, a new video output requirement (MP4 in/out with audio, Tenor/Klipy → GIF, still-only effects), and the animated-images rule extended to video frames.
- `safety`: a new requirement for how untrusted video is passed to ffmpeg (forced MP4 demuxer, local files only, time and size bounds).
- `deployment`: `FFMPEG_PATH` configuration, and ffmpeg in the OCI image and dev shell.

## Impact

- Code: `internal/imaging` gains video decode and encode through ffmpeg, and `Image` remembers its video source (frame rate, audio, GIF-like). `internal/imagefind` returns input sources with their kind instead of bare URLs, and reads gifv/video embeds. In `internal/modules/notsobot`, the ImageMagick-backed effects run per frame for video. Config gains `FFMPEG_PATH`, and `main` probes ffmpeg at startup.
- Container image grows by roughly the ffmpeg-headless closure. This is measured during implementation and reported.
- Heavier jobs: up to 150 frames per video through effects. They stay within the existing 3 minute limit and the per-server queue.
- Tests: MP4 fixtures (a short clip with audio and a Klipy-style silent clip) and ffmpeg-backed tests that skip when ffmpeg is missing, like the ImageMagick tests.
- Docs: README (video support, `FFMPEG_PATH`) and CLAUDE.md (video pipeline, ffmpeg safety).
