# Spec Delta

## ADDED Requirements

### Requirement: ffmpeg input handling
Untrusted video MUST be passed to ffmpeg only as a local temporary file, with the MP4 demuxer forced and only local file and pipe protocols allowed, so input can't select other demuxers or make ffmpeg open network or other files. ffmpeg SHALL run without reading stdin and be stopped when the command's time limit expires, and decoding SHALL be bounded by the video limits (10 seconds, 15 fps, 512 px).

#### Scenario: Playlist pointing elsewhere
- **WHEN** a user supplies a file that is really an HLS playlist referencing other URLs
- **THEN** it is rejected as not an MP4 and ffmpeg never fetches anything

#### Scenario: Hanging decode
- **WHEN** ffmpeg takes longer than the command's time limit
- **THEN** it is killed and the user is told processing took too long
