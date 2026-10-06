# Tasks

## 1. Video fixtures and packaging

- [x] 1.1 Add `ffmpeg-headless` to the dev shell and OCI image (with `FFMPEG_PATH` in the image env), and add `FFMPEG_PATH` (default `ffmpeg`) to config; record the image size before and after; verify `nix develop -c ffmpeg -version` works and `nix build .#image` succeeds
- [x] 1.2 Create small test fixtures in `internal/imaging/testdata/`: a ~2 s 320×240 MP4 with an audio track (generated with ffmpeg test sources), a silent Klipy-style MP4, a MOV (QuickTime brand) and a WebM; verify each with ffprobe and keep them under ~200 KB total

## 2. Imaging: sniffing, decode, encode

- [x] 2.1 Extend `Sniff`/`Decode` to recognise MP4 (`ftyp` with a non-QuickTime major brand) as video and to reject MOV/WebM with "only MP4 videos are supported"; verify with unit tests on the fixtures and that existing image sniffing tests still pass
- [x] 2.2 Add an ffmpeg runner (probe at startup via `-version`; `exec.CommandContext`; `-nostdin`; `-f mov`; `-protocol_whitelist file,pipe`; per-job temp dir removed afterwards) and `DecodeVideo` producing frames at min(source, 15) fps, longest side ≤ 512 px, first 10 s, plus the audio track for non-GIF-like sources; verify with tests (skipped without ffmpeg) for frame count/size limits on the fixture, audio extracted, and context cancellation killing ffmpeg
- [x] 2.3 Add `VideoInfo` to `Image`, preserved by `Map` and frame-building helpers, and make `Encode` produce MP4 (libx264 yuv420p, even dimensions, audio copied with AAC fallback, `-shortest`, faststart) for video, GIF for GIF-like video, PNG/GIF otherwise; verify by round-tripping the fixtures: MP4 with audio → MP4 that ffprobe shows with video + audio and ≤10 s, GIF-like → animated GIF with FPS-based delays, odd dimensions handled

## 3. Finding video inputs

- [x] 3.1 Change `imagefind` to return sources with a kind (Image, Video, GIFVideo): MP4 attachments, `video`/`gifv` embeds via `video.url`, `.mp4` links, Tenor links resolved to MP4 (API `mp4` format or `og:video`); Tenor and Klipy hosts always GIFVideo; verify with unit tests for embed and attachment classification, Tenor `og:video` parsing against a saved fixture, and that existing image discovery tests still pass
- [x] 3.2 Re-fetch the invoking message (up to 3 times, ~1 s apart) when it contains a Tenor or Klipy link without an embed yet, then use its gifv embed; verify with a unit test using a fake message source that returns the embed on the second fetch

## 4. Effects on video

- [x] 4.1 Update `load` to decode video sources (refusing them up front with "video support needs ffmpeg" when ffmpeg is unavailable) and confirm pure-Go effects (invert, flop, flip, rotate, pixelate, mirrors, solarize, swap*, deepfry, shake, iascii) produce MP4 for MP4 input and GIF for GIF-like input; verify with fixture tests for invert (MP4 → MP4 with audio, GIF-like → GIF)
- [x] 4.2 Run the ImageMagick-backed effects (magik, gmagik, caption, watermark, wall, watercolor) per frame for video through a shared helper; verify with fixture tests (skipped without magick/ffmpeg) that gmagik on the MP4 fixture yields an MP4 with audio and frame count matching the decode, and on the GIF-like fixture yields a GIF
- [x] 4.3 Make still-only effects (jpeg, vw, merge, petimg, triggered, glitch) use the first video frame and return a still; verify with tests for jpeg and glitch on the MP4 fixture

## 5. Docs and checks

- [x] 5.1 Update README.md (MP4 input, Tenor/Klipy behaviour, limits, `FFMPEG_PATH`) and CLAUDE.md (video pipeline, ffmpeg safety rules, `Image.Video`); verify by reading both sections
- [x] 5.2 Run `golangci-lint run ./...` (0 issues), `go test ./...` and `nix flake check`; verify all pass, the image builds, and report its size change
- [x] 5.3 Live check: run gmagik and invert end to end on a real Klipy MP4 and a real Tenor MP4 (throwaway test, not committed); then manually in Discord run `/gmagik` on an MP4 upload with sound, on a Tenor GIF and on a Klipy GIF, and verify MP4-with-audio and GIF outputs respectively
