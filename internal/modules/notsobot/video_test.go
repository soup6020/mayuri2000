package notsobot

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/imagefind"
	"mayuri2000/internal/imaging"
)

func videoModule(t *testing.T) *Module {
	t.Helper()
	ffPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	ff, err := imaging.NewFFmpeg(context.Background(), ffPath)
	if err != nil {
		t.Fatal(err)
	}
	mg := &imaging.Magick{}
	if p, err := exec.LookPath("magick"); err == nil {
		mg, _ = imaging.NewMagick(context.Background(), p)
	}
	m := New(imagefind.New(""), mg, ff)
	m.Commands() // registers the effects map
	return m
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../imaging/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// runOn decodes fixture bytes as the given kind and runs the named effect.
func runOn(t *testing.T, m *Module, name, file string, kind imagefind.Kind, args map[string]any) result {
	t.Helper()
	c, _ := cmd.NewTestContext(nil, args)
	raw := fixture(t, file)
	img, err := m.decodeBytes(c, raw, kind)
	if err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	res, err := m.effects[name](c, raw, img)
	if err != nil {
		t.Fatalf("%s on %s: %v", name, file, err)
	}
	return res
}

// streams returns ffprobe's codec types, e.g. "video,audio".
func streams(t *testing.T, data []byte) string {
	t.Helper()
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH")
	}
	f, _ := os.CreateTemp(t.TempDir(), "out-*.mp4")
	_, _ = f.Write(data)
	_ = f.Close()
	out, err := exec.CommandContext(t.Context(), probe, "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", f.Name()).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", ",")
}

func TestPureEffectsOnVideo(t *testing.T) {
	m := videoModule(t)
	res := runOn(t, m, "invert", "clip_audio.mp4", imagefind.Video, nil)
	if res.ext != "mp4" || imaging.Sniff(res.data) != "mp4" || streams(t, res.data) != "video,audio" {
		t.Fatalf("invert MP4: ext=%s streams=%s", res.ext, streams(t, res.data))
	}
	res = runOn(t, m, "invert", "gifv.mp4", imagefind.GIFVideo, nil)
	if res.ext != "gif" || imaging.Sniff(res.data) != "gif" {
		t.Fatalf("invert GIF-like: ext=%s", res.ext)
	}
	// Shake keeps a video a video (it previously always emitted GIF).
	if res := runOn(t, m, "shake", "clip_audio.mp4", imagefind.Video, nil); res.ext != "mp4" {
		t.Fatalf("shake MP4: ext=%s", res.ext)
	}
	if res := runOn(t, m, "iascii", "clip_audio.mp4", imagefind.Video, nil); res.ext != "mp4" {
		t.Fatalf("iascii MP4: ext=%s", res.ext)
	}
	// An unknown link whose content is MP4 is still decoded as video.
	if res := runOn(t, m, "flop", "clip_audio.mp4", imagefind.Image, nil); res.ext != "mp4" {
		t.Fatalf("flop on MP4 content: ext=%s", res.ext)
	}
}

func TestVideoWithoutFFmpeg(t *testing.T) {
	m := New(imagefind.New(""), &imaging.Magick{}, &imaging.FFmpeg{})
	c, _ := cmd.NewTestContext(nil, nil)
	// Refused before any download (the URL is unreachable on purpose).
	_, _, err := m.decodeSource(c, imagefind.Source{URL: "https://example.invalid/clip.mp4", Kind: imagefind.Video})
	if err == nil || !strings.Contains(err.Error(), "Video support needs ffmpeg") {
		t.Fatalf("got %v", err)
	}
	// Content sniffed as MP4 from a plain link is refused the same way.
	if _, err := m.decodeBytes(c, fixture(t, "clip_audio.mp4"), imagefind.Image); err == nil || !strings.Contains(err.Error(), "needs ffmpeg") {
		t.Fatalf("got %v", err)
	}
	// Other containers are rejected clearly.
	if _, err := m.decodeBytes(c, fixture(t, "clip.webm"), imagefind.Image); err == nil || err.Error() != "Only MP4 videos are supported" {
		t.Fatalf("got %v", err)
	}
}

func magickOrSkip(t *testing.T, m *Module) {
	t.Helper()
	if !m.magick.Available || !m.magick.LiquidRescale {
		t.Skip("ImageMagick with liblqr not available")
	}
}

func frameCount(t *testing.T, data []byte) int {
	t.Helper()
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH")
	}
	f, _ := os.CreateTemp(t.TempDir(), "out-*.mp4")
	_, _ = f.Write(data)
	_ = f.Close()
	out, err := exec.CommandContext(t.Context(), probe, "-v", "error", "-count_frames", "-select_streams", "v:0",
		"-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", f.Name()).Output()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range strings.TrimSpace(string(out)) {
		n = n*10 + int(r-'0')
	}
	return n
}

func TestMagickEffectsOnVideo(t *testing.T) {
	m := videoModule(t)
	magickOrSkip(t, m)

	c, _ := cmd.NewTestContext(nil, nil)
	decoded, err := m.decodeBytes(c, fixture(t, "clip_audio.mp4"), imagefind.Video)
	if err != nil {
		t.Fatal(err)
	}
	res := runOn(t, m, "gmagik", "clip_audio.mp4", imagefind.Video, nil)
	if res.ext != "mp4" || streams(t, res.data) != "video,audio" {
		t.Fatalf("gmagik MP4: ext=%s streams=%s", res.ext, streams(t, res.data))
	}
	if got, want := frameCount(t, res.data), len(decoded.Frames); got != want {
		t.Fatalf("gmagik frames = %d, want %d (one per decoded frame)", got, want)
	}
	if res := runOn(t, m, "gmagik", "gifv.mp4", imagefind.GIFVideo, nil); res.ext != "gif" {
		t.Fatalf("gmagik GIF-like: ext=%s", res.ext)
	}
	// magik routes video to the per-frame path too.
	if res := runOn(t, m, "magik", "gifv.mp4", imagefind.GIFVideo, nil); res.ext != "gif" {
		t.Fatalf("magik GIF-like: ext=%s", res.ext)
	}
	for _, name := range []string{"caption", "watermark", "wall", "watercolor"} {
		if res := runOn(t, m, name, "clip_audio.mp4", imagefind.Video, nil); res.ext != "mp4" || streams(t, res.data) != "video,audio" {
			t.Errorf("%s MP4: ext=%s", name, res.ext)
		}
	}
}

func TestStillEffectsOnVideo(t *testing.T) {
	m := videoModule(t)
	if res := runOn(t, m, "jpeg", "clip_audio.mp4", imagefind.Video, nil); res.ext != "jpg" || imaging.Sniff(res.data) != "jpeg" {
		t.Fatalf("jpeg on MP4: ext=%s", res.ext)
	}
	for _, kind := range []imagefind.Kind{imagefind.Video, imagefind.GIFVideo} {
		res := runOn(t, m, "glitch", "clip_audio.mp4", kind, map[string]any{"iterations": 5, "amount": 10, "seed": 10})
		if f := imaging.Sniff(res.data); f != "png" && f != "jpeg" {
			t.Fatalf("glitch on video (kind %v) produced %q (%s)", kind, f, res.ext)
		}
		if res.content == "" {
			t.Errorf("glitch on video should report its parameters")
		}
	}
	for _, name := range []string{"petimg", "triggered", "vw"} {
		res := runOn(t, m, name, "clip_audio.mp4", imagefind.Video, nil)
		if res.ext == "mp4" {
			t.Errorf("%s should produce a still/template output, got mp4", name)
		}
	}
}
