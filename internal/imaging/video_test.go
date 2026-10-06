package imaging

import (
	"context"
	"errors"
	"image"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSniffVideo(t *testing.T) {
	for name, want := range map[string]string{
		"clip_audio.mp4": "mp4", "gifv.mp4": "mp4", "clip.mov": "mov", "clip.webm": "webm",
	} {
		data := readFixture(t, name)
		if got := Sniff(data); got != want {
			t.Errorf("Sniff(%s) = %q, want %q", name, got, want)
		}
		if !IsVideo(Sniff(data)) {
			t.Errorf("%s not reported as video", name)
		}
	}
	// Image decoding refuses video with the right error.
	if _, err := Decode(readFixture(t, "clip_audio.mp4")); !errors.Is(err, ErrVideoInput) {
		t.Errorf("Decode(mp4) = %v, want ErrVideoInput", err)
	}
	for _, name := range []string{"clip.mov", "clip.webm"} {
		if _, err := Decode(readFixture(t, name)); !errors.Is(err, ErrUnsupportedVideo) || err.Error() != "only MP4 videos are supported" {
			t.Errorf("Decode(%s) = %v", name, err)
		}
	}
	// ImageMagick never receives video.
	if _, err := InputSpec(readFixture(t, "clip_audio.mp4")); err == nil {
		t.Error("InputSpec accepted an MP4")
	}
	if _, err := Decode([]byte("#EXTM3U\nhttp://example.com/a.ts\n")); !errors.Is(err, ErrNotImage) {
		t.Errorf("playlist: %v", err)
	}
}

func ffmpegOrSkip(t *testing.T) *FFmpeg {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	f, err := NewFFmpeg(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// ffprobeStreams returns "video,audio" style codec types and the duration.
func ffprobeStreams(t *testing.T, data []byte) (string, float64) {
	t.Helper()
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH")
	}
	f, _ := os.CreateTemp(t.TempDir(), "probe-*.mp4")
	_, _ = f.Write(data)
	_ = f.Close()
	out, err := exec.CommandContext(t.Context(), path, "-v", "error", "-show_entries", "stream=codec_type:format=duration", "-of", "csv=p=0", f.Name()).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var types []string
	var dur float64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if d, err := strconv.ParseFloat(line, 64); err == nil {
			dur = d
		} else {
			types = append(types, line)
		}
	}
	return strings.Join(types, ","), dur
}

func TestDecodeVideo(t *testing.T) {
	ff := ffmpegOrSkip(t)
	ctx := context.Background()

	img, err := DecodeVideo(ctx, ff, readFixture(t, "clip_audio.mp4"), false)
	if err != nil {
		t.Fatal(err)
	}
	// 2 s at 30 fps is capped to 15 fps → ~30 frames; 320×240 is under 512.
	if n := len(img.Frames); n < 28 || n > 32 {
		t.Errorf("frames = %d, want ~30", n)
	}
	if b := img.First().Rect; b.Dx() != 320 || b.Dy() != 240 {
		t.Errorf("size = %v", b)
	}
	if img.Video == nil || img.Video.FPS != 15 || len(img.Video.Audio) == 0 || img.Video.GIFLike {
		t.Fatalf("video info = %+v", img.Video)
	}
	if img.Delays[0] != 7 {
		t.Errorf("delay = %d, want 7 (15 fps)", img.Delays[0])
	}

	gif, err := DecodeVideo(ctx, ff, readFixture(t, "gifv.mp4"), true)
	if err != nil {
		t.Fatal(err)
	}
	if gif.Video.Audio != nil || !gif.Video.GIFLike || gif.Video.FPS != 15 {
		t.Fatalf("gif-like info = %+v", gif.Video)
	}

	for _, name := range []string{"clip.mov", "clip.webm"} {
		if _, err := DecodeVideo(ctx, ff, readFixture(t, name), false); !errors.Is(err, ErrUnsupportedVideo) {
			t.Errorf("DecodeVideo(%s) = %v", name, err)
		}
	}
	if _, err := DecodeVideo(ctx, &FFmpeg{}, readFixture(t, "clip_audio.mp4"), false); !errors.Is(err, ErrFFmpegUnavailable) {
		t.Errorf("without ffmpeg: %v", err)
	}
}

func TestDecodeVideoLimits(t *testing.T) {
	ff := ffmpegOrSkip(t)
	// A 14 s 1280×720 60 fps clip must come out as ≤10 s, ≤15 fps, ≤512 px.
	dir := t.TempDir()
	long := dir + "/long.mp4"
	if out, err := exec.CommandContext(t.Context(), ff.Path, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=60:duration=14",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "45", "-pix_fmt", "yuv420p", long).CombinedOutput(); err != nil {
		t.Fatalf("making long clip: %v %s", err, out)
	}
	data, _ := os.ReadFile(long)
	img, err := DecodeVideo(context.Background(), ff, data, false)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(img.Frames); n > 151 || n < 145 {
		t.Errorf("frames = %d, want ~150 (10 s × 15 fps)", n)
	}
	if b := img.First().Rect; b.Dx() != 512 || b.Dy() != 288 {
		t.Errorf("size = %v, want 512x288", b)
	}
}

func TestDecodeVideoCancelled(t *testing.T) {
	ff := ffmpegOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)
	if _, err := DecodeVideo(ctx, ff, readFixture(t, "clip_audio.mp4"), false); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
}

func TestEncodeVideo(t *testing.T) {
	ff := ffmpegOrSkip(t)
	ctx := context.Background()

	// MP4 with audio → MP4 with video and audio, ≤10 s.
	img, err := DecodeVideo(ctx, ff, readFixture(t, "clip_audio.mp4"), false)
	if err != nil {
		t.Fatal(err)
	}
	data, ext, err := Invert(img).EncodeContext(ctx)
	if err != nil || ext != "mp4" || Sniff(data) != "mp4" {
		t.Fatalf("encode: ext=%s err=%v", ext, err)
	}
	types, dur := ffprobeStreams(t, data)
	if types != "video,audio" || dur <= 1.5 || dur > 10.1 {
		t.Fatalf("streams=%q duration=%v", types, dur)
	}

	// Odd dimensions are handled (yuv420p needs even sizes).
	odd := img.Map(func(f *image.NRGBA) *image.NRGBA { return Rotate(&Image{Frames: []*image.NRGBA{f}}, 33).First() })
	if b := odd.First().Rect; b.Dx()%2 == 0 && b.Dy()%2 == 0 {
		odd = img.Map(func(f *image.NRGBA) *image.NRGBA {
			return ToNRGBA(f.SubImage(image.Rect(0, 0, 319, 239)))
		})
	}
	if data, _, err := odd.EncodeContext(ctx); err != nil || Sniff(data) != "mp4" {
		t.Fatalf("odd dimensions: %v", err)
	}

	// GIF-like → animated GIF with FPS-based delays.
	gifLike, err := DecodeVideo(ctx, ff, readFixture(t, "gifv.mp4"), true)
	if err != nil {
		t.Fatal(err)
	}
	data, ext, err = Invert(gifLike).EncodeContext(ctx)
	if err != nil || ext != "gif" {
		t.Fatalf("gif-like encode: ext=%s err=%v", ext, err)
	}
	out, err := Decode(data)
	if err != nil || !out.Animated() || out.Delays[0] != 7 {
		t.Fatalf("gif-like output: %v animated=%v delays=%v", err, out != nil && out.Animated(), out.Delays[:1])
	}

	// Plain images still encode as before.
	_, still := stillFixture(t)
	if _, ext, _ := Invert(still).EncodeContext(ctx); ext != "png" {
		t.Fatalf("still ext = %s", ext)
	}
}
