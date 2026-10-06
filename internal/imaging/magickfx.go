package imaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"mayuri2000/assets"
)

// Magik applies liquid rescaling ("content aware scale") to a still image.
// scale controls the seam carving delta, as in NotSoBot's do_magik.
func (m *Magick) Magik(ctx context.Context, data []byte, scale int) ([]byte, error) {
	in, err := InputSpec(data)
	if err != nil {
		return nil, err
	}
	d1, d2 := max(1, scale/2), max(1, scale)
	return m.Run(ctx, data, in+"[0]",
		"-resize", "800x800>",
		"-liquid-rescale", fmt.Sprintf("50x50%%+%d+0", d1),
		"-liquid-rescale", fmt.Sprintf("150x150%%+%d+0", d2),
		"-depth", "8", "png:-")
}

// GMagik applies liquid rescaling to every frame of an animation, or turns a
// still into a 30 frame progressively melting animation.
func (m *Magick) GMagik(ctx context.Context, data []byte, src *Image) ([]byte, error) {
	in, err := InputSpec(data)
	if err != nil {
		return nil, err
	}
	if src.Animated() {
		return m.Run(ctx, data, in,
			"-coalesce",
			"-resize", "512x512>",
			"-liquid-rescale", "50x50%+1+0",
			"-liquid-rescale", "150x150%+2+0",
			"-layers", "optimize", "-loop", "0", "gif:-")
	}
	w, h := src.First().Rect.Dx(), src.First().Rect.Dy()
	if w > 512 || h > 512 {
		if w > h {
			w, h = 512, max(1, h*512/w)
		} else {
			w, h = max(1, w*512/h), 512
		}
	}
	size := fmt.Sprintf("%dx%d!", w, h)
	args := []string{in + "[0]", "-resize", size}
	for i := 0; i < 29; i++ {
		args = append(args, "(", "+clone",
			"-liquid-rescale", "75x75%+1+0",
			"-liquid-rescale", "125x125%+2+0",
			"-resize", size, ")")
	}
	args = append(args, "-set", "delay", "10", "-loop", "0", "-layers", "optimize", "gif:-")
	return m.Run(ctx, data, args...)
}

var colorRe = regexp.MustCompile(`^(#?[0-9a-fA-F]{3,8}|[a-zA-Z]{3,24})$`)

// ParseColor validates a colour name or hex code for ImageMagick.
func ParseColor(s string) (string, bool) {
	if !colorRe.MatchString(s) {
		return "", false
	}
	if isHex(s) && !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	return s, true
}

func isHex(s string) bool {
	s = strings.TrimPrefix(s, "#")
	_, err := strconv.ParseUint(s, 16, 64)
	return err == nil && (len(s) == 3 || len(s) == 4 || len(s) == 6 || len(s) == 8)
}

// escapeText neutralises ImageMagick's special handling of text arguments:
// a leading '@' reads a file and '%' starts a property escape.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", "%%")
	if strings.HasPrefix(s, "@") {
		s = `\` + s
	}
	return s
}

var (
	fontFileOnce sync.Once
	fontFile     string
	fontFileErr  error
)

// arialPath writes the embedded arial font to a temp file once so magick can
// read it.
func arialPath() (string, error) {
	fontFileOnce.Do(func() {
		data, err := assets.FS.ReadFile("fonts/arial.ttf")
		if err != nil {
			fontFileErr = err
			return
		}
		f, err := os.CreateTemp("", "mayuri-arial-*.ttf")
		if err != nil {
			fontFileErr = err
			return
		}
		_, err = f.Write(data)
		if err = errors.Join(err, f.Close()); err != nil {
			fontFileErr = err
			return
		}
		fontFile = f.Name()
	})
	return fontFile, fontFileErr
}

// CaptionOpts positions caption text. X and Y are percentages of the image
// width and height.
type CaptionOpts struct {
	Text  string
	Color string
	Size  int
	X, Y  int
}

// Caption draws wrapped text onto every frame.
func (m *Magick) Caption(ctx context.Context, data []byte, src *Image, o CaptionOpts) ([]byte, string, error) {
	in, err := InputSpec(data)
	if err != nil {
		return nil, "", err
	}
	font, err := arialPath()
	if err != nil {
		return nil, "", err
	}
	w, h := src.First().Rect.Dx(), src.First().Rect.Dy()
	left, top := w*o.X/100, h*o.Y/100
	args := []string{in}
	if !src.Animated() {
		args[0] += "[0]"
	}
	args = append(args,
		"-coalesce", "null:",
		"(", "-size", fmt.Sprintf("%dx", max(1, w-left)), "-background", "none",
		"-fill", o.Color, "-font", font, "-pointsize", strconv.Itoa(o.Size),
		"caption:"+escapeText(o.Text), ")",
		"-geometry", fmt.Sprintf("+%d+%d", left, top),
		"-layers", "composite")
	return m.finish(ctx, data, src, args)
}

// WatermarkOpts positions a watermark. X and Y are percentages of the image
// size and Transparency is 0-100.
type WatermarkOpts struct {
	Mark         []byte // nil for the default mark
	X, Y         int
	Transparency int
}

// Watermark overlays a mark (by default the brazzers logo) onto every frame.
func (m *Magick) Watermark(ctx context.Context, data []byte, src *Image, o WatermarkOpts) ([]byte, string, error) {
	in, err := InputSpec(data)
	if err != nil {
		return nil, "", err
	}
	mark := o.Mark
	if mark == nil {
		if mark, err = assets.FS.ReadFile("templates/brazzers.png"); err != nil {
			return nil, "", err
		}
	}
	markSpec, err := InputSpec(mark)
	if err != nil {
		return nil, "", err
	}
	f, err := os.CreateTemp("", "mayuri-mark-*")
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(mark)
	if err = errors.Join(err, f.Close()); err != nil {
		return nil, "", err
	}
	markSpec = strings.TrimSuffix(markSpec, "-") + f.Name() + "[0]"

	w, h := src.First().Rect.Dx(), src.First().Rect.Dy()
	// NotSoBot shrank the base to ~256px; scale the mark instead so its
	// relative size matches without destroying the base image.
	markW := max(1, w*150/256)
	opacity := float64(100-o.Transparency) / 100
	args := []string{in}
	if !src.Animated() {
		args[0] += "[0]"
	}
	args = append(args,
		"-coalesce", "null:",
		"(", markSpec, "-resize", fmt.Sprintf("%dx", markW),
		"-alpha", "set", "-channel", "A", "-evaluate", "multiply", strconv.FormatFloat(opacity, 'f', 2, 64), "+channel", ")",
		"-geometry", fmt.Sprintf("+%d+%d", w*o.X/100, h*o.Y/100),
		"-layers", "composite")
	return m.finish(ctx, data, src, args)
}

func (m *Magick) finish(ctx context.Context, data []byte, src *Image, args []string) ([]byte, string, error) {
	if src.Animated() {
		out, err := m.Run(ctx, data, append(args, "-layers", "optimize", "-loop", "0", "gif:-")...)
		return out, "gif", err
	}
	out, err := m.Run(ctx, data, append(args, "-depth", "8", "png:-")...)
	return out, "png", err
}

// Wall tiles the image across a wall receding into the distance.
func (m *Magick) Wall(ctx context.Context, data []byte, src *Image) ([]byte, string, error) {
	in, err := InputSpec(data)
	if err != nil {
		return nil, "", err
	}
	w, h := src.First().Rect.Dx(), src.First().Rect.Dy()
	// Tiles are 128px tall and keep the source's aspect ratio.
	tw := max(48, min(256, 128*w/max(1, h)))
	f := float64(tw) / 128
	p := func(x, y float64) string { return fmt.Sprintf("%.0f,%.0f", x, y) }
	points := strings.Join([]string{
		"0,0", p(0, -40),
		fmt.Sprintf("%d,0", tw), p(170*f, 20),
		"0,128", p(0, 440),
		fmt.Sprintf("%d,128", tw), p(170*f, 380),
	}, " ")
	args := []string{in}
	if !src.Animated() {
		args[0] += "[0]"
	}
	args = append(args,
		"-coalesce", "-resize", fmt.Sprintf("%dx128!", tw),
		"-virtual-pixel", "tile", "-background", "none",
		"-set", "option:distort:viewport", "640x400", "+repage",
		"-distort", "Perspective", points, "+repage")
	return m.finish(ctx, data, src, args)
}

// WatercolorOpts mirrors upstream NotSoBot's watercolor parameters.
type WatercolorOpts struct {
	Contrast  float64 // -100..100, default 0
	Edge      float64 // edge gain, default 2
	Mixing    int     // percent of ink edges mixed in, default 33
	Smoothing int     // extra texture smoothing, default 0
}

// Watercolor paints the image with Kuwahara smoothing and inked edges.
// Animations are processed frame by frame.
func (m *Magick) Watercolor(ctx context.Context, src *Image, o WatercolorOpts) ([]byte, string, error) {
	args := []string{
		"png:-",
		"(", "-clone", "0", "-kuwahara", strconv.Itoa(3 + o.Smoothing), "-modulate", "100,120", ")",
		"(", "-clone", "0", "-colorspace", "gray", "-blur", "0x1", "-edge", "1", "-negate",
		"-evaluate", "pow", strconv.FormatFloat(o.Edge, 'f', 2, 64), ")",
		"-delete", "0",
		"(", "-clone", "0", "-clone", "1", "-compose", "multiply", "-composite", ")",
		"-delete", "1",
		"-compose", "blend", "-define", "compose:args=" + strconv.Itoa(o.Mixing), "-composite",
		"-brightness-contrast", "0x" + strconv.FormatFloat(o.Contrast, 'f', 1, 64),
		"-depth", "8", "png:-",
	}
	out := &Image{Delays: src.Delays, Format: src.Format, Video: src.Video}
	for _, f := range src.Frames {
		frame, err := EncodePNG(f)
		if err != nil {
			return nil, "", err
		}
		res, err := m.Run(ctx, frame, args...)
		if err != nil {
			return nil, "", err
		}
		img, err := Decode(res)
		if err != nil {
			return nil, "", err
		}
		// Keep the original transparency; the filters don't preserve it.
		painted := img.First()
		for i := 3; i < len(painted.Pix) && i < len(f.Pix); i += 4 {
			painted.Pix[i] = f.Pix[i]
		}
		out.Frames = append(out.Frames, painted)
	}
	return out.EncodeContext(ctx)
}

// LiquidFrame applies gmagik's per-frame liquid rescale to one PNG frame.
func (m *Magick) LiquidFrame(ctx context.Context, frame []byte) ([]byte, error) {
	return m.Run(ctx, frame, "png:-[0]",
		"-resize", "512x512>",
		"-liquid-rescale", "50x50%+1+0",
		"-liquid-rescale", "150x150%+2+0",
		"-depth", "8", "png:-")
}
