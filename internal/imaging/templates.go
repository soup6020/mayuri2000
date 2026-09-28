package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	dimg "github.com/disintegration/imaging"

	"mayuri2000/assets"
)

var (
	assetMu    sync.Mutex
	assetCache = map[string]*image.NRGBA{}
)

// Asset loads and caches an embedded image by path.
func Asset(path string) *image.NRGBA {
	assetMu.Lock()
	defer assetMu.Unlock()
	if img, ok := assetCache[path]; ok {
		return img
	}
	data, err := assets.FS.ReadFile(path)
	if err != nil {
		panic("missing embedded asset " + path)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		panic("bad embedded asset " + path + ": " + err.Error())
	}
	n := ToNRGBA(img)
	assetCache[path] = n
	return n
}

// Petpet makes the classic 10-frame petting hand GIF, ported from
// NotSoBot's petpet_maker.
func Petpet(src *Image) ([]byte, error) {
	const res, frames = 128, 10
	base := dimg.Resize(src.First(), res, res, dimg.Lanczos)
	var out []*image.NRGBA
	var delays []int
	for i := 0; i < frames; i++ {
		squeeze := float64(i)
		if i >= frames/2 {
			squeeze = float64(frames - i)
		}
		width := 0.8 + squeeze*0.02
		height := 0.8 - squeeze*0.05
		offX := (1-width)*0.5 + 0.1
		offY := (1 - height) - 0.08

		canvas := image.NewNRGBA(image.Rect(0, 0, res, res))
		scaled := dimg.Resize(base, int(math.Round(width*res)), int(math.Round(height*res)), dimg.Lanczos)
		pos := image.Pt(int(math.Round(offX*res)), int(math.Round(offY*res)))
		draw.Draw(canvas, scaled.Rect.Add(pos), scaled, image.Point{}, draw.Over)
		hand := dimg.Resize(Asset(fmt.Sprintf("petpet/pet%d.gif", i)), res, res, dimg.Lanczos)
		draw.Draw(canvas, canvas.Rect, hand, image.Point{}, draw.Over)
		out = append(out, canvas)
		delays = append(delays, 2)
	}
	return EncodeGIF(out, delays)
}

// Triggered makes the shaking "TRIGGERED" GIF from trigger_image.
func Triggered(src *Image) ([]byte, error) {
	top := dimg.Resize(src.First(), 640, 640, dimg.Lanczos)
	banner := Asset("templates/triggered.jpg")
	offsets := []image.Point{{-60, -60}, {-45, -50}, {-50, -45}, {-45, -65}}
	var frames []*image.NRGBA
	var delays []int
	for _, off := range offsets {
		f := image.NewNRGBA(image.Rect(0, 0, 512, 660))
		draw.Draw(f, f.Rect, image.Black, image.Point{}, draw.Src)
		draw.Draw(f, top.Rect.Add(off), top, image.Point{}, draw.Over)
		draw.Draw(f, banner.Rect.Add(image.Pt(0, 572)), banner, image.Point{}, draw.Src)
		frames = append(frames, f)
		delays = append(delays, 2)
	}
	return EncodeGIF(frames, delays)
}

// RIP draws text on a tombstone.
func RIP(text string) ([]byte, error) {
	tmpl := Asset("templates/rip.jpg")
	img := image.NewNRGBA(tmpl.Rect)
	draw.Draw(img, img.Rect, tmpl, image.Point{}, draw.Src)
	face := Face("arial.ttf", 35)
	lines := strings.Join(Wrap(text, 15), "\n")
	shadow := color.NRGBA{50, 50, 50, 255}
	for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		drawCenteredOffset(img, face, 400, 50, d, shadow, lines)
	}
	drawCenteredOffset(img, face, 400, 50, image.Point{}, color.NRGBA{105, 105, 105, 255}, lines)
	return EncodeJPEG(img, 90)
}

func drawCenteredOffset(dst *image.NRGBA, face fontFace, width, y int, d image.Point, c color.Color, text string) {
	lh := LineHeight(face)
	for i, line := range strings.Split(text, "\n") {
		x := (width-TextWidth(face, line))/2 + d.X
		DrawText(dst, face, x, y+i*lh+d.Y, c, line)
	}
}

// Achievement renders a Minecraft "Achievement get!" toast.
func Achievement(text string) ([]byte, error) {
	if r := []rune(text); len(r) > 20 {
		text = string(r[:20]) + " ..."
	}
	tmpl := Asset("templates/achievement.png")
	img := image.NewNRGBA(tmpl.Rect)
	draw.Draw(img, img.Rect, tmpl, image.Point{}, draw.Src)
	DrawText(img, Face("Minecraftia.ttf", 17), 60, 30, color.White, text)
	return EncodePNG(img)
}
