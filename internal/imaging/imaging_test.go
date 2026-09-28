package imaging

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func fixture(w, h int, shift uint8) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x*255/w) + shift, uint8(y*255/h) + shift, 128, 255})
		}
	}
	return img
}

func stillFixture(t *testing.T) ([]byte, *Image) {
	t.Helper()
	raw, err := EncodePNG(fixture(200, 150, 0))
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, img
}

func gifFixture(t *testing.T) ([]byte, *Image) {
	t.Helper()
	raw, err := EncodeGIF([]*image.NRGBA{fixture(120, 90, 0), fixture(120, 90, 60), fixture(120, 90, 120)}, []int{10, 10, 10})
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !img.Animated() || len(img.Frames) != 3 {
		t.Fatalf("gif fixture decoded to %d frames", len(img.Frames))
	}
	return raw, img
}

// check decodes output and saves it under $IMAGING_OUT for eyeballing.
func check(t *testing.T, name string, data []byte, wantAnimated bool) *Image {
	t.Helper()
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("%s: output does not decode: %v", name, err)
	}
	if img.Animated() != wantAnimated {
		t.Fatalf("%s: animated = %v, want %v", name, img.Animated(), wantAnimated)
	}
	if b := img.First().Rect; b.Dx() < 8 || b.Dy() < 8 {
		t.Fatalf("%s: suspicious output size %v", name, b)
	}
	if dir := os.Getenv("IMAGING_OUT"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, name+"."+Sniff(data)), data, 0o644)
	}
	return img
}

func TestPureEffects(t *testing.T) {
	raw, still := stillFixture(t)
	_, anim := gifFixture(t)

	for _, tc := range []struct {
		name string
		src  *Image
		fn   func(*Image) *Image
	}{
		{"flip", still, Flip},
		{"flop", still, Flop},
		{"invert", still, Invert},
		{"rotate", still, func(m *Image) *Image { return Rotate(m, 45) }},
		{"pixelate", still, func(m *Image) *Image { return Pixelate(m, 9) }},
		{"waaw", still, func(m *Image) *Image { return Mirror(m, Waaw) }},
		{"haah", still, func(m *Image) *Image { return Mirror(m, Haah) }},
		{"woow", still, func(m *Image) *Image { return Mirror(m, Woow) }},
		{"hooh", still, func(m *Image) *Image { return Mirror(m, Hooh) }},
		{"invert_gif", anim, Invert},
		{"pixelate_gif", anim, func(m *Image) *Image { return Pixelate(m, 5) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _, err := tc.fn(tc.src).Encode()
			if err != nil {
				t.Fatal(err)
			}
			check(t, tc.name, data, tc.src.Animated())
		})
	}

	t.Run("mirror_dims", func(t *testing.T) {
		out := Mirror(still, Waaw).First()
		if out.Rect.Dx() != 200 || out.Rect.Dy() != 150 {
			t.Fatalf("waaw changed size to %v", out.Rect)
		}
	})
	t.Run("merge", func(t *testing.T) {
		data, _ := EncodePNG(Merge([]*Image{still, anim}, true))
		out := check(t, "merge", data, false)
		if out.First().Rect.Dx() != 120 {
			t.Fatalf("merge width = %d, want 120", out.First().Rect.Dx())
		}
	})
	t.Run("jpeg", func(t *testing.T) {
		data, _ := EncodeJPEG(still.First(), 1)
		check(t, "jpeg", data, false)
	})
	t.Run("petpet", func(t *testing.T) {
		data, err := Petpet(still)
		if err != nil {
			t.Fatal(err)
		}
		check(t, "petpet", data, true)
	})
	t.Run("triggered", func(t *testing.T) {
		data, err := Triggered(still)
		if err != nil {
			t.Fatal(err)
		}
		check(t, "triggered", data, true)
	})
	t.Run("rip", func(t *testing.T) {
		data, err := RIP("someone's\nHopes and Dreams")
		if err != nil {
			t.Fatal(err)
		}
		check(t, "rip", data, false)
	})
	t.Run("achievement", func(t *testing.T) {
		data, err := Achievement("Wrote a Discord bot in Go")
		if err != nil {
			t.Fatal(err)
		}
		check(t, "achievement", data, false)
	})
	t.Run("vaporwave", func(t *testing.T) {
		data, err := EncodePNG(Vaporwave(fixture(800, 600, 0), "hello"))
		if err != nil {
			t.Fatal(err)
		}
		check(t, "vaporwave", data, false)
	})
	t.Run("glitch", func(t *testing.T) {
		data, ext, err := Glitch(context.Background(), nil, raw, still, 10, 10, 20)
		if err != nil || ext != "jpg" {
			t.Fatal(ext, err)
		}
		check(t, "glitch", data, false)
	})
	t.Run("glitch_gif", func(t *testing.T) {
		graw, ganim := gifFixture(t)
		data, ext, err := Glitch(context.Background(), nil, graw, ganim, 10, 10, 20)
		if err != nil || ext != "gif" {
			t.Fatal(ext, err)
		}
		check(t, "glitch_gif", data, true)
	})
	t.Run("iascii", func(t *testing.T) {
		data, _, err := ASCIIArt(still, 60)
		if err != nil {
			t.Fatal(err)
		}
		check(t, "iascii", data, false)
	})
	t.Run("gascii", func(t *testing.T) {
		data, _, err := ASCIIArt(anim, 40)
		if err != nil {
			t.Fatal(err)
		}
		check(t, "gascii", data, true)
	})
}

func TestFiglet(t *testing.T) {
	art, err := Figlet("Hi")
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split([]byte(art), []byte("\n"))
	if len(lines) < 6 || len(lines) > 7 {
		t.Fatalf("expected 7 lines of starwars figlet, got %d:\n%s", len(lines), art)
	}
	t.Logf("\n%s", art)
	if _, err := Figlet("héllo"); err == nil {
		t.Fatal("expected error for unsupported characters")
	}
	long, _ := Figlet("this is a much longer sentence")
	for _, l := range bytes.Split([]byte(long), []byte("\n")) {
		if len([]rune(string(l))) > 80 {
			t.Fatalf("line exceeds 80 columns: %q", l)
		}
	}
}

func TestDecodeGIFDisposal(t *testing.T) {
	// Second frame only covers a sub-rectangle; decoded frames must be full.
	pal := color.Palette{color.Transparent, color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}}
	f1 := image.NewPaletted(image.Rect(0, 0, 20, 20), pal)
	for i := range f1.Pix {
		f1.Pix[i] = 1
	}
	f2 := image.NewPaletted(image.Rect(5, 5, 10, 10), pal)
	for i := range f2.Pix {
		f2.Pix[i] = 2
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{f1, f2}, Delay: []int{5, 5}, Config: image.Config{Width: 20, Height: 20, ColorModel: pal}}); err != nil {
		t.Fatal(err)
	}
	img, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	f := img.Frames[1]
	if f.Rect.Dx() != 20 || f.NRGBAAt(0, 0).R != 255 || f.NRGBAAt(6, 6).B != 255 {
		t.Fatalf("frame 2 not composed correctly: %v %v %v", f.Rect, f.NRGBAAt(0, 0), f.NRGBAAt(6, 6))
	}
}

func TestSniffRejectsUnknown(t *testing.T) {
	if _, err := Decode([]byte("<svg xmlns=...>")); err == nil {
		t.Fatal("expected svg to be rejected")
	}
	if _, err := InputSpec([]byte("push graphic-context")); err == nil {
		t.Fatal("expected MVG to be rejected")
	}
}

func TestEscapeText(t *testing.T) {
	for in, want := range map[string]string{
		"@/etc/passwd": `\@/etc/passwd`,
		"100%":         "100%%",
		`a\nb`:         `a\\nb`,
		"hi @you":      "hi @you",
	} {
		if got := escapeText(in); got != want {
			t.Errorf("escapeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string]string{"white": "white", "ff0000": "#ff0000", "#abc": "#abc"} {
		if got, ok := ParseColor(in); !ok || got != want {
			t.Errorf("ParseColor(%q) = %q, %v", in, got, ok)
		}
	}
	for _, bad := range []string{"rgb(1,2,3)", "@file", "", "white; rm"} {
		if _, ok := ParseColor(bad); ok {
			t.Errorf("ParseColor(%q) accepted", bad)
		}
	}
}

func magickOrSkip(t *testing.T) *Magick {
	t.Helper()
	path, err := exec.LookPath("magick")
	if err != nil {
		t.Skip("magick not on PATH")
	}
	m, err := NewMagick(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMagickEffects(t *testing.T) {
	m := magickOrSkip(t)
	ctx := context.Background()
	raw, still := stillFixture(t)
	graw, anim := gifFixture(t)

	if m.LiquidRescale {
		t.Run("magik", func(t *testing.T) {
			data, err := m.Magik(ctx, raw, 2)
			if err != nil {
				t.Fatal(err)
			}
			check(t, "magik", data, false)
		})
		t.Run("gmagik_still", func(t *testing.T) {
			data, err := m.GMagik(ctx, raw, still)
			if err != nil {
				t.Fatal(err)
			}
			out := check(t, "gmagik_still", data, true)
			if len(out.Frames) != 30 {
				t.Fatalf("got %d frames, want 30", len(out.Frames))
			}
		})
		t.Run("gmagik_gif", func(t *testing.T) {
			data, err := m.GMagik(ctx, graw, anim)
			if err != nil {
				t.Fatal(err)
			}
			check(t, "gmagik_gif", data, true)
		})
	}
	t.Run("glitch_magick", func(t *testing.T) {
		data, ext, err := Glitch(ctx, m, raw, still, 10, 10, 20)
		if err != nil || ext != "png" {
			t.Fatal(ext, err)
		}
		check(t, "glitch_magick", data, false)
	})
	t.Run("caption", func(t *testing.T) {
		data, ext, err := m.Caption(ctx, raw, still, CaptionOpts{Text: "@/etc/passwd 100% hello", Color: "yellow", Size: 20, X: 10, Y: 50})
		if err != nil || ext != "png" {
			t.Fatal(ext, err)
		}
		check(t, "caption", data, false)
	})
	t.Run("caption_gif", func(t *testing.T) {
		data, ext, err := m.Caption(ctx, graw, anim, CaptionOpts{Text: "moving", Color: "#ff0000", Size: 20})
		if err != nil || ext != "gif" {
			t.Fatal(ext, err)
		}
		check(t, "caption_gif", data, true)
	})
	t.Run("watermark", func(t *testing.T) {
		data, ext, err := m.Watermark(ctx, raw, still, WatermarkOpts{X: 50, Y: 70, Transparency: 20})
		if err != nil || ext != "png" {
			t.Fatal(ext, err)
		}
		check(t, "watermark", data, false)
	})
	t.Run("watermark_gif_custom", func(t *testing.T) {
		data, ext, err := m.Watermark(ctx, graw, anim, WatermarkOpts{Mark: raw})
		if err != nil || ext != "gif" {
			t.Fatal(ext, err)
		}
		check(t, "watermark_gif_custom", data, true)
	})
}
