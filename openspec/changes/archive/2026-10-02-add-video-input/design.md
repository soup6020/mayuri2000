# Design

## Context

Current pipeline (`internal/modules/notsobot`): `load` gets one URL from `imagefind.Finder.Find`, downloads it through the safe client (25 MiB cap), and decodes it with `imaging.Decode` into `*imaging.Image` (fully composed NRGBA frames plus GIF delays). Each effect receives both the raw bytes and the decoded image.

- **Pure-Go effects** transform frames with `Image.Map` and encode with `Image.Encode()` (GIF if animated, else PNG).
- **ImageMagick-backed effects** (magik, gmagik, caption, watermark, wall) pass the *raw bytes* to the `magick` CLI with a sniffed input coder. Watercolor already runs magick per frame on PNG-encoded frames.
- **Glitch** on a GIF scrambles the raw GIF's colour table.

`imagefind` returns bare URLs. It reads attachments with `image/*` content types, `image` embeds, image links, custom emoji and avatars. It resolves Tenor view links to their *GIF* rendition via the Tenor API or by scraping the page.

Observed provider behaviour (checked live):
- Klipy pages (`klipy.com/gifs/...`) return 403 to non-browser clients. Their media lives on `static2.klipy.com` as `.mp4`, `.gif` and `.webp`, and the MP4 downloads fine. A sampled Klipy MP4 was H.264 854×480 at 25 fps, 4.8 s, *with* an AAC track.
- Tenor view pages expose `og:video` (`media.tenor.com/...AAPo/...mp4`) next to the GIF.
- When a Tenor or Klipy GIF is posted from Discord's picker, the message gets a `gifv` embed whose `video.url` is the provider's MP4. Embeds are often added a moment after the message is created.

nixpkgs `ffmpeg-headless` 9.0.1 provides the `mov,mp4` demuxer and the `libx264` and `aac` encoders. Its closure is roughly 300 MB uncompressed; part of it is shared with ImageMagick.

## Goals / Non-Goals

**Goals:**
- One video path in `internal/imaging` that every effect benefits from, with the output format following the input (MP4 → MP4 with audio; Tenor/Klipy → GIF).
- Tenor and Klipy posts work from history, replies and context menus, without scraping Klipy or needing API keys.

**Non-Goals:**
- MOV, WebM, MKV, animated AVIF and other containers. Audio-only inputs.
- Processing beyond 10 s, above 15 fps or above 512 px. Keeping the original resolution.
- Video-specific effects (speed, reverse, audio filters).

## Decisions

**Input sources carry a kind.**
`imagefind` returns `[]Source{URL, Kind}` instead of `[]string`, with kinds `Image`, `Video` and `GIFVideo` (Tenor or Klipy MP4). The kind comes from where the URL was found:
- `gifv` embeds and resolved Tenor links are `GIFVideo`.
- `video/mp4` attachments, `video` embeds and `.mp4` links are `Video`, unless their host is `media.tenor.com` or a `klipy.com` subdomain, which are always `GIFVideo`.

Embeds use `video.url` (the provider's original URL, fetched through the safe client) rather than Discord's proxy. Alternative considered: deciding GIF-ness from the file itself (e.g. no audio). Rejected because the sampled Klipy MP4 has an audio track.

**Own-message embeds.**
When the invoking prefix message contains a Tenor or Klipy link but no embed yet, the finder re-fetches the message (`GetMessage`) up to 3 times, about 1 s apart, and uses the gifv embed once it appears. Slash commands keep using history, replies and targets, where embeds already exist. Tenor links without an embed still resolve directly (API, or `og:video` scraping) as a fallback.

**`Image` remembers its video source.**
`imaging.Image` gains an optional `Video *VideoInfo{FPS float64, Audio []byte, GIFLike bool}`. `Map` and every helper that builds a new `Image` from an old one preserve it. `Encode()` chooses the output:
- **MP4** (`video/mp4`, extension `mp4`) when `Video != nil && !GIFLike`.
- **GIF** when animated (including GIF-like video).
- **PNG** otherwise.

Effects keep calling `encoded(img)`, so pure-Go effects need no per-effect changes. Effects that build a still from `First()` drop `Video` and return a still as today.

**Decoding with ffmpeg.**
`imaging.DecodeVideo(ctx, ff, data, kind)` works in four steps:
1. Write the bytes to a temp file (MP4 often isn't streamable from a pipe).
2. Probe it with ffmpeg (`-f mov -i in.mp4`, parsing the stream summary) to confirm a video stream exists and read its frame rate. The output rate is the source rate or 15 fps, whichever is lower.
3. Decode frames with `ffmpeg -nostdin -protocol_whitelist file,pipe -f mov -i in.mp4 -t 10 -an -vf "fps=<rate>,scale=512:512:force_original_aspect_ratio=decrease:flags=lanczos" -f image2pipe -c:v png -`, then read the PNG stream into frames.
4. Unless the source is GIF-like, extract audio with `-vn -t 10 -c:a copy -f mp4` into a second temp file, and keep it only if it contains a stream.

A failed demux maps to "only MP4 videos are supported". Before calling ffmpeg, the sniffer accepts `ftyp` boxes whose major brand isn't QuickTime (`qt  `), so MOV, WebM, HLS playlists and anything else are rejected before reaching ffmpeg.

**Encoding with ffmpeg.**
For MP4 output, pipe frames as raw RGBA (`-f rawvideo -pix_fmt rgba -s WxH -r FPS -i -`) plus the audio temp file, and encode with `-c:v libx264 -pix_fmt yuv420p -crf 23 -preset veryfast -c:a copy -shortest -movflags +faststart`. Dimensions are padded to even numbers (yuv420p needs this) by scaling by one pixel if needed. If copying the audio fails (an unusual codec in the MP4), retry with `-c:a aac`. Effects that change frame size (wall, rotate) encode at the new size, and frame count follows the effect (shake keeps one output frame per input frame on animated input). GIF output for GIF-like videos uses the existing GIF encoder, with delays from FPS (e.g. 15 fps → 7/100 s).

**ImageMagick-backed effects on video run per frame.**
Those effects take raw bytes. For video, a helper encodes each frame as PNG, runs the effect's still-image path, and reassembles the frames with the original `VideoInfo`:
- magik and gmagik apply the per-frame liquid rescale used for animated GIFs.
- caption, watermark and wall apply their still operation per frame.
- watercolor already works this way.

Alternative considered: converting the video to a GIF and reusing the GIF paths. Rejected because it quantises to 256 colours before the effect and would degrade MP4 output. Per-frame process start-up (about 150 processes) is acceptable within the 3 minute limit and the global worker bound.

**Still-only and byte-level effects.**
jpeg, vw, merge, petimg, triggered, rip-style templates and glitch use the first frame. Glitch's GIF path corrupts the raw GIF colour table, which has no MP4 equivalent, so on video it glitches the first frame as a still JPEG.

**ffmpeg discovery and safety.**
`FFMPEG_PATH` (default `ffmpeg`) is probed at startup with `-version`. If it's missing, video sources fail with the "video support needs ffmpeg" user error before download, so image use is unaffected. Every ffmpeg run:
- uses `exec.CommandContext` with the command's context (killed at the 3 minute heavy-command limit),
- uses `-nostdin` (except when piping frames in for encoding),
- forces `-f mov` with `-protocol_whitelist file,pipe`,
- reads and writes only the per-job temp directory, which is removed afterwards.

**Packaging.**
Add `pkgs.ffmpeg-headless` to the OCI image contents (with `FFMPEG_PATH` set in the image env) and to the dev shell. Record the image size before and after in the task notes. If the increase is unacceptable, a trimmed ffmpeg build is a follow-up rather than part of this change.

## Risks / Trade-offs

- [Image size grows substantially] → Measured and reported; a trimmed ffmpeg is a possible follow-up.
- [Long processing: 150 frames × magick per frame] → 15 fps and 512 px caps, the existing per-server queue and global worker bound, and the 3 minute timeout with a clear "took too long" message.
- [Output exceeds the upload limit (GIF output for long Tenor clips)] → Existing size check and message. GIF frames are capped at 150 and 512 px.
- [Embeds arrive late for links in the command message] → Bounded re-fetch (about 3 s). After that, Tenor falls back to direct resolution and Klipy reports that no image was found.
- [ffmpeg parsing untrusted input] → Forced demuxer, protocol whitelist, sniffing before ffmpeg, temp-file isolation, timeouts, and the existing 25 MiB download cap.
- [Audio/video length mismatch after the fps change] → `-shortest` plus the same 10 s cut on both streams.

## Migration Plan

1. Deploy the new image; it includes ffmpeg. No configuration change is needed.
2. Rollback: deploy the previous image. Nothing is persisted, so there's no data migration.
