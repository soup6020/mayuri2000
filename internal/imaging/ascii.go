package imaging

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"
	"sync"

	dimg "github.com/disintegration/imaging"

	"mayuri2000/assets"
)

type figletFont struct {
	hardblank rune
	height    int
	chars     map[rune][][]rune
}

var (
	starwarsOnce sync.Once
	starwars     *figletFont
)

func loadFiglet(path string) (*figletFont, error) {
	data, err := assets.FS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	if !sc.Scan() {
		return nil, errors.New("empty figlet font")
	}
	header := strings.Fields(sc.Text())
	if len(header) < 6 || !strings.HasPrefix(header[0], "flf2a") {
		return nil, errors.New("not a figlet font")
	}
	f := &figletFont{hardblank: []rune(header[0])[5], chars: map[rune][][]rune{}}
	f.height, _ = strconv.Atoi(header[1])
	comments, _ := strconv.Atoi(header[5])
	for i := 0; i < comments && sc.Scan(); i++ {
	}
	for ch := rune(32); ch <= 126; ch++ {
		glyph := make([][]rune, 0, f.height)
		for i := 0; i < f.height && sc.Scan(); i++ {
			line := strings.TrimRight(sc.Text(), " \r")
			if line != "" {
				end := line[len(line)-1:]
				line = strings.TrimRight(line, end)
			}
			glyph = append(glyph, []rune(line))
		}
		// Pad rows to equal width.
		w := 0
		for _, row := range glyph {
			w = max(w, len(row))
		}
		for i := range glyph {
			for len(glyph[i]) < w {
				glyph[i] = append(glyph[i], ' ')
			}
		}
		f.chars[ch] = glyph
	}
	return f, nil
}

func smushChar(l, r, hardblank rune) rune {
	switch {
	case l == ' ':
		return r
	case r == ' ':
		return l
	case l == hardblank || r == hardblank:
		return 0
	case l == r: // rule 1: equal character
		return l
	}
	const underscoreable = "|/\\[]{}()<>"
	if l == '_' && strings.ContainsRune(underscoreable, r) { // rule 2
		return r
	}
	if r == '_' && strings.ContainsRune(underscoreable, l) {
		return l
	}
	classes := []string{"|", "/\\", "[]", "{}", "()", "<>"} // rule 3: hierarchy
	cl, cr := -1, -1
	for i, c := range classes {
		if strings.ContainsRune(c, l) {
			cl = i
		}
		if strings.ContainsRune(c, r) {
			cr = i
		}
	}
	if cl >= 0 && cr >= 0 && cl != cr {
		if cl > cr {
			return l
		}
		return r
	}
	switch string([]rune{l, r}) { // rule 4: opposite pair
	case "[]", "][", "{}", "}{", "()", ")(":
		return '|'
	}
	return 0
}

func (f *figletFont) smushAmount(lines [][]rune, glyph [][]rune) int {
	amt := 1 << 30
	for row := 0; row < f.height; row++ {
		line, g := lines[row], glyph[row]
		lb := len(line) - 1
		for lb >= 0 && line[lb] == ' ' {
			lb--
		}
		gb := 0
		for gb < len(g) && g[gb] == ' ' {
			gb++
		}
		a := gb + len(line) - 1 - lb
		l, r := ' ', ' '
		if lb >= 0 {
			l = line[lb]
		}
		if gb < len(g) {
			r = g[gb]
		}
		if l == ' ' || r == ' ' || smushChar(l, r, f.hardblank) != 0 {
			a++
		}
		amt = min(amt, a)
	}
	return max(0, min(amt, len(glyph[0])))
}

// Render renders text, wrapping output blocks at width columns.
func (f *figletFont) Render(text string, width int) (string, error) {
	var blocks []string
	for _, para := range strings.Split(text, "\n") {
		lines := make([][]rune, f.height)
		flush := func() {
			var b strings.Builder
			for _, l := range lines {
				b.WriteString(strings.TrimRight(strings.ReplaceAll(string(l), string(f.hardblank), " "), " "))
				b.WriteByte('\n')
			}
			blocks = append(blocks, strings.TrimRight(b.String(), "\n"))
			lines = make([][]rune, f.height)
		}
		for _, ch := range para {
			glyph, ok := f.chars[ch]
			if !ok {
				return "", errors.New("unsupported character")
			}
			amt := f.smushAmount(lines, glyph)
			if len(lines[0])+len(glyph[0])-amt > width && len(lines[0]) > 0 {
				flush()
				amt = 0
			}
			for row := range lines {
				line := lines[row]
				for i := 0; i < amt; i++ {
					pos := len(line) - amt + i
					if pos < 0 {
						continue
					}
					if s := smushChar(line[pos], glyph[row][i], f.hardblank); s != 0 {
						line[pos] = s
					}
				}
				lines[row] = append(line, glyph[row][amt:]...)
			}
		}
		if len(lines[0]) > 0 {
			flush()
		}
	}
	return strings.Join(blocks, "\n"), nil
}

// Figlet renders text with the "starwars" figlet font.
func Figlet(text string) (string, error) {
	starwarsOnce.Do(func() {
		var err error
		if starwars, err = loadFiglet("fonts/starwars.flf"); err != nil {
			panic(err)
		}
	})
	return starwars.Render(text, 80)
}

// RenderText draws monospaced text onto an image.
func RenderText(text string, fg, bg color.Color, size float64) *image.NRGBA {
	face := Face("FreeMonoBold.ttf", size)
	lines := strings.Split(text, "\n")
	lh := LineHeight(face)
	w := 0
	for _, l := range lines {
		w = max(w, TextWidth(face, l))
	}
	const pad = 20
	img := image.NewNRGBA(image.Rect(0, 0, w+2*pad, lh*len(lines)+2*pad))
	draw.Draw(img, img.Rect, image.NewUniform(bg), image.Point{}, draw.Src)
	for i, l := range lines {
		DrawText(img, face, pad, pad+i*lh, fg, l)
	}
	return img
}

const asciiRamp = "@%#*+=-:. "

// ImageToASCII converts a frame to ASCII art with the given column count.
func ImageToASCII(img *image.NRGBA, cols int) string {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	cols = max(1, min(cols, w))
	// Monospace cells are roughly twice as tall as they are wide.
	rows := max(1, int(float64(cols)*float64(h)/float64(w)*0.5))
	small := dimg.Resize(img, cols, rows, dimg.Box)
	var b strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := small.NRGBAAt(x, y)
			// Composite onto white so transparent areas render blank.
			a := float64(c.A) / 255
			l := (0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B))*a + 255*(1-a)
			idx := int(l / 256 * float64(len(asciiRamp)))
			b.WriteByte(asciiRamp[min(idx, len(asciiRamp)-1)])
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// ASCIIArt renders img (or up to 20 frames of an animation) as an image of
// ASCII art, the replacement for NotSoBot's aalib-based iascii/gascii.
func ASCIIArt(m *Image, cols int) ([]byte, string, error) {
	frames := m.Frames
	if len(frames) > 20 {
		frames = frames[:20]
	}
	out := &Image{Delays: m.Delays}
	for _, f := range frames {
		out.Frames = append(out.Frames, RenderText(ImageToASCII(f, cols), color.Black, color.White, 12))
	}
	return out.Encode()
}
