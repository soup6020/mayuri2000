package imaging

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mayuri2000/internal/sandbox"
)

// Video input limits: the first MaxVideoSeconds, at most MaxVideoFPS, scaled
// so the longest side is at most MaxVideoSide.
const (
	MaxVideoSeconds = 10
	MaxVideoFPS     = 15.0
	MaxVideoSide    = 512
)

var (
	// ErrVideoInput is returned by Decode for MP4 data, which needs DecodeVideo.
	ErrVideoInput = errors.New("this is an MP4 video, not an image")
	// ErrUnsupportedVideo is returned for video containers other than MP4.
	ErrUnsupportedVideo = errors.New("only MP4 videos are supported")
	// ErrFFmpegUnavailable means video can't be processed on this host.
	ErrFFmpegUnavailable = errors.New("video support needs ffmpeg, which isn't available")
)

// VideoInfo is attached to an Image decoded from a video so encoding can
// produce a matching video (or GIF) again.
type VideoInfo struct {
	FPS float64
	// Audio is the source's audio track as an MP4 (m4a) file, or nil.
	Audio []byte
	// GIFLike marks Tenor/Klipy GIFs: decoded from MP4 but output as GIF.
	GIFLike bool
	ff      *FFmpeg
}

// FFmpeg runs the ffmpeg CLI.
type FFmpeg struct {
	Path      string
	Available bool
}

// NewFFmpeg probes the ffmpeg binary.
func NewFFmpeg(ctx context.Context, path string) (*FFmpeg, error) {
	f := &FFmpeg{Path: path}
	if _, err := runSandboxed(ctx, nil, nil, path, "-hide_banner", "-version"); err != nil {
		return f, fmt.Errorf("running %s -version: %w", path, err)
	}
	f.Available = true
	return f, nil
}

// inputArgs force the MP4 demuxer and restrict ffmpeg to local files and
// pipes, so untrusted input can't select another demuxer or reach the
// network or other files.
func inputArgs(path string) []string {
	return []string{"-protocol_whitelist", "file,pipe", "-f", "mov", "-i", path}
}

// command runs ffmpeg in the decoder sandbox with dir (the job's own temp
// directory, holding its input and output files) as the only writable path.
func (f *FFmpeg) command(ctx context.Context, dir string, args ...string) (*exec.Cmd, error) {
	cmd, err := sandbox.Command(ctx, dir, f.Path, append([]string{"-hide_banner", "-loglevel", "error"}, args...)...)
	if err != nil {
		return nil, err
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}

func (f *FFmpeg) run(ctx context.Context, dir string, stdin io.Reader, args ...string) ([]byte, error) {
	cmd, err := f.command(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffmpeg timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

var (
	probeVideoRe = regexp.MustCompile(`Stream #\S+.*: Video: .*?(\d+(?:\.\d+)?) (?:fps|tbr)`)
	probeHasVid  = regexp.MustCompile(`Stream #\S+.*: Video: `)
	probeHasAud  = regexp.MustCompile(`Stream #\S+.*: Audio: `)
)

type probeResult struct {
	fps      float64
	hasAudio bool
}

// probe inspects in.mp4. ffmpeg with no output exits non-zero after
// printing the stream summary, which is what we parse.
func (f *FFmpeg) probe(ctx context.Context, path string) (probeResult, error) {
	cmd, err := sandbox.Command(ctx, filepath.Dir(path), f.Path, append([]string{"-hide_banner", "-nostdin"}, inputArgs(path)...)...)
	if err != nil {
		return probeResult{}, err
	}
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	if ctx.Err() != nil {
		return probeResult{}, fmt.Errorf("ffmpeg timed out: %w", ctx.Err())
	}
	out := stderr.String()
	if !probeHasVid.MatchString(out) {
		return probeResult{}, ErrUnsupportedVideo
	}
	r := probeResult{fps: MaxVideoFPS, hasAudio: probeHasAud.MatchString(out)}
	if m := probeVideoRe.FindStringSubmatch(out); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
			r.fps = v
		}
	}
	return r, nil
}

// DecodeVideo decodes MP4 data into frames within the video limits, keeping
// the audio track unless the source is GIF-like.
func DecodeVideo(ctx context.Context, f *FFmpeg, data []byte, gifLike bool) (*Image, error) {
	if f == nil || !f.Available {
		return nil, ErrFFmpegUnavailable
	}
	switch Sniff(data) {
	case "mp4":
	case "mov", "webm":
		return nil, ErrUnsupportedVideo
	default:
		return nil, ErrNotImage
	}
	dir, err := os.MkdirTemp("", "mayuri-video-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.mp4")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, err
	}

	pr, err := f.probe(ctx, in)
	if err != nil {
		return nil, err
	}
	fps := math.Min(pr.fps, MaxVideoFPS)
	// Fit within MaxVideoSide without ever enlarging smaller videos.
	scale := fmt.Sprintf("scale=w='min(iw,%d)':h='min(ih,%d)':force_original_aspect_ratio=decrease:flags=lanczos", MaxVideoSide, MaxVideoSide)
	args := append([]string{"-nostdin"}, inputArgs(in)...)
	args = append(args, "-t", strconv.Itoa(MaxVideoSeconds), "-an",
		"-vf", fmt.Sprintf("fps=%s,%s", strconv.FormatFloat(fps, 'f', 3, 64), scale),
		"-f", "image2pipe", "-c:v", "png", "pipe:1")
	out, err := f.run(ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	frames, err := readPNGStream(out)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, ErrUnsupportedVideo
	}
	maxFrames := int(math.Ceil(MaxVideoSeconds*fps)) + 1
	if len(frames) > maxFrames {
		frames = frames[:maxFrames]
	}

	info := &VideoInfo{FPS: fps, GIFLike: gifLike, ff: f}
	if pr.hasAudio && !gifLike {
		audio := filepath.Join(dir, "audio.m4a")
		aargs := append([]string{"-nostdin"}, inputArgs(in)...)
		aargs = append(aargs, "-t", strconv.Itoa(MaxVideoSeconds), "-vn", "-c:a", "copy", "-f", "mp4", audio)
		if _, err := f.run(ctx, dir, nil, aargs...); err == nil {
			if b, err := os.ReadFile(audio); err == nil && len(b) > 0 {
				info.Audio = b
			}
		}
	}

	delay := max(2, int(math.Round(100/fps)))
	delays := make([]int, len(frames))
	for i := range delays {
		delays[i] = delay
	}
	return &Image{Frames: frames, Delays: delays, Format: "mp4", Video: info}, nil
}

// readPNGStream splits ffmpeg's image2pipe PNG output into frames.
func readPNGStream(data []byte) ([]*image.NRGBA, error) {
	r := bufio.NewReader(bytes.NewReader(data))
	var frames []*image.NRGBA
	for {
		if _, err := r.Peek(1); err == io.EOF {
			return frames, nil
		}
		img, err := png.Decode(r)
		if err != nil {
			return nil, fmt.Errorf("reading decoded frames: %w", err)
		}
		frames = append(frames, ToNRGBA(img))
	}
}

// encodeMP4 encodes frames at the video's FPS with its audio track.
func encodeMP4(ctx context.Context, m *Image) ([]byte, error) {
	v := m.Video
	if v.ff == nil || !v.ff.Available {
		return nil, ErrFFmpegUnavailable
	}
	dir, err := os.MkdirTemp("", "mayuri-video-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// Raw video needs one size; effects keep frame sizes uniform, but
	// normalise defensively. Transparency is flattened onto black.
	b := m.First().Rect
	w, h := b.Dx(), b.Dy()
	var raw bytes.Buffer
	raw.Grow(w * h * 4 * len(m.Frames))
	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	for _, fr := range m.Frames {
		draw.Draw(canvas, canvas.Rect, image.Black, image.Point{}, draw.Src)
		draw.Draw(canvas, canvas.Rect, fr, fr.Rect.Min, draw.Over)
		raw.Write(canvas.Pix)
	}

	out := filepath.Join(dir, "out.mp4")
	args := []string{
		"-protocol_whitelist", "file,pipe",
		"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", w, h),
		"-framerate", strconv.FormatFloat(v.FPS, 'f', 3, 64), "-i", "pipe:0",
	}
	audioArgs := func(codec string) []string { return nil }
	if len(v.Audio) > 0 {
		audio := filepath.Join(dir, "audio.m4a")
		if err := os.WriteFile(audio, v.Audio, 0o600); err != nil {
			return nil, err
		}
		args = append(args, "-f", "mov", "-i", audio)
		audioArgs = func(codec string) []string {
			return []string{"-map", "0:v", "-map", "1:a", "-c:a", codec, "-shortest"}
		}
	}
	encode := func(audioCodec string) ([]byte, error) {
		a := append(append([]string{}, args...), audioArgs(audioCodec)...)
		a = append(a,
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
			"-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "23", "-preset", "veryfast",
			"-movflags", "+faststart", "-f", "mp4", "-y", out)
		if _, err := v.ff.run(ctx, dir, bytes.NewReader(raw.Bytes()), a...); err != nil {
			return nil, err
		}
		return os.ReadFile(out)
	}
	data, err := encode("copy")
	if err != nil && len(v.Audio) > 0 && ctx.Err() == nil {
		// An audio codec MP4 can't carry as-is: re-encode it.
		data, err = encode("aac")
	}
	return data, err
}
