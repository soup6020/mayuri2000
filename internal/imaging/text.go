package imaging

import (
	"image"
	"image/color"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"mayuri2000/assets"
)

type fontFace = font.Face

var (
	fontsMu sync.Mutex
	fonts   = map[string]*opentype.Font{}
)

// Face returns a font face for one of the embedded fonts (e.g. "arial.ttf").
func Face(name string, size float64) font.Face {
	fontsMu.Lock()
	f, ok := fonts[name]
	if !ok {
		data, err := assets.FS.ReadFile("fonts/" + name)
		if err != nil {
			panic("missing embedded font " + name)
		}
		f, err = opentype.Parse(data)
		if err != nil {
			panic("bad embedded font " + name + ": " + err.Error())
		}
		fonts[name] = f
	}
	fontsMu.Unlock()
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return face
}

// TextWidth returns the advance width of s in pixels.
func TextWidth(face font.Face, s string) int {
	return font.MeasureString(face, s).Ceil()
}

// LineHeight returns the recommended line spacing of face in pixels.
func LineHeight(face font.Face) int {
	return face.Metrics().Height.Ceil()
}

// DrawText draws s with its top-left corner at (x, y).
func DrawText(dst *image.NRGBA, face font.Face, x, y int, c color.Color, s string) {
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y+face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(s)
}

// DrawCentered draws each line of text centered horizontally within width,
// starting at y.
func DrawCentered(dst *image.NRGBA, face font.Face, width, y int, c color.Color, text string) {
	lh := LineHeight(face)
	for i, line := range strings.Split(text, "\n") {
		x := (width - TextWidth(face, line)) / 2
		DrawText(dst, face, x, y+i*lh, c, line)
	}
}

// Wrap breaks text into lines of at most width runes, breaking on spaces
// where possible (like Python's textwrap.wrap).
func Wrap(text string, width int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		var cur []rune
		for _, word := range strings.Fields(para) {
			w := []rune(word)
			for len(w) > width {
				if len(cur) > 0 {
					lines = append(lines, string(cur))
					cur = nil
				}
				lines = append(lines, string(w[:width]))
				w = w[width:]
			}
			switch {
			case len(cur) == 0:
				cur = w
			case len(cur)+1+len(w) <= width:
				cur = append(append(cur, ' '), w...)
			default:
				lines = append(lines, string(cur))
				cur = w
			}
		}
		if len(cur) > 0 {
			lines = append(lines, string(cur))
		}
	}
	return lines
}
