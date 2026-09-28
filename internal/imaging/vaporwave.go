package imaging

import (
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"math"
	"math/rand/v2"
	"path"
	"sort"
	"strconv"
	"strings"

	dimg "github.com/disintegration/imaging"

	"mayuri2000/assets"
)

func vwAssets(dir string) []string {
	entries, err := fs.ReadDir(assets.FS, "vw/"+dir)
	if err != nil {
		panic(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, path.Join("vw", dir, e.Name()))
	}
	sort.Strings(out)
	return out
}

// seeded returns a deterministic RNG for the given seed text and salt, the
// equivalent of Python's Random(seedvalue + salt).
func seeded(seed, salt string) *rand.Rand {
	h := sha256.Sum256([]byte(seed + salt))
	return rand.New(rand.NewPCG(binary.LittleEndian.Uint64(h[:8]), binary.LittleEndian.Uint64(h[8:16])))
}

func pick(r *rand.Rand, items []string) *image.NRGBA {
	return Asset(items[r.IntN(len(items))])
}

func vwColor(k float64) color.NRGBA {
	clamp := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }
	return color.NRGBA{clamp(math.Mod(k, 255)), clamp(255 * math.Cos(k)), clamp(255 * (1 - math.Sin(k))), 255}
}

// Vaporwave is a port of macintoshplus.draw_method1.
func Vaporwave(src *image.NRGBA, name string) *image.NRGBA {
	seed := name
	if name == "vapor wave" {
		seed = strconv.Itoa(rand.IntN(1_000_000))
	}
	windows, pics, greek, bubbles := vwAssets("windows"), vwAssets("pics"), vwAssets("greek"), vwAssets("bubbles")

	im := image.NewNRGBA(src.Rect)
	draw.Draw(im, im.Rect, image.Black, image.Point{}, draw.Src)
	draw.Draw(im, im.Rect, src, image.Point{}, draw.Over)

	insertCascade(im, pick(seeded(seed, "0"), windows), 0.5, 100, 100)
	paste(im, pick(seeded(seed, "1"), pics), 500, 500, true)
	paste(im, pick(seeded(seed, "3"), pics), 500, 0, true)
	paste(im, pics2(pics), rand.IntN(im.Rect.Dy()+1), rand.IntN(im.Rect.Dx()+1), true)
	paste(im, pick(seeded(seed, "3"), greek), 0, 450, true)
	insertScatter(im, pick(seeded(seed, "7"), pics), 44)
	bubble := pick(seeded(seed, "5"), bubbles)
	paste(im, bubble, im.Rect.Dx()-bubble.Rect.Dx(), im.Rect.Dy()-bubble.Rect.Dy(), true)

	// NotSoBot always drew text at y=500, which is off-canvas for small
	// images; keep it on screen.
	textY := min(500, im.Rect.Dy()*2/3)
	// The embedded arial has no CJK or full-width glyphs, so the Japanese
	// corpus line would render as boxes; draw the name letter-spaced instead.
	vwText(im, name, float64(100+seeded(seed, "13").IntN(401)), 50, textY)

	out := smooth(im)
	return saturate(out, float64(3+seeded(seed, "").IntN(8)))
}

func pics2(pics []string) *image.NRGBA { return Asset(pics[rand.IntN(len(pics))]) }

func paste(dst, fg *image.NRGBA, x, y int, masked bool) {
	op := draw.Src
	if masked {
		op = draw.Over
	}
	draw.Draw(dst, fg.Rect.Add(image.Pt(x, y)), fg, image.Point{}, op)
}

func insertCascade(im, fg *image.NRGBA, k float64, x, y int) {
	acc, v := -1.0, 0.0
	for i := 0; i < int(k*100); i++ {
		fi := float64(i)
		dy := v*fi + 0.5*acc*fi*fi
		v += acc * fi
		pos := image.Pt(x+11*i, int(float64(y)-dy))
		paste(im, fg, pos.X, pos.Y, false)
		if im.Rect.Dy()-fg.Rect.Dy() <= pos.Y+fg.Rect.Dy() {
			v = -v
			acc *= 0.9
		}
	}
}

func insertScatter(im, fg *image.NRGBA, k float64) {
	bw, bh := float64(im.Rect.Dx()), float64(im.Rect.Dy())
	fw, fh := float64(fg.Rect.Dx()), float64(fg.Rect.Dy())
	ratio := fw / fh
	for i := 500; i < 600; i++ {
		fi := float64(i)
		x := int((bw-fw)/2 + fi*math.Sin(fi)/math.Sin(k))
		y := int((bh-fh)/2 + fi*math.Cos(fi)/ratio/math.Cos(k))
		paste(im, fg, x, y, true)
	}
}

// spaced renders the vaporwave "ａｅｓｔｈｅｔｉｃ" look with a font lacking
// full-width glyphs by spacing the letters out.
func spaced(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func vwText(im *image.NRGBA, txt string, k float64, x, y int) {
	size := 1.0
	target := 0.5 * float64(im.Rect.Dx()) * 0.7
	for size < 400 && float64(TextWidth(Face("arial.ttf", size), txt)) < target {
		size++
	}
	face := Face("arial.ttf", size)
	txt = spaced(txt)
	DrawText(im, face, x-2, y-2, vwColor(k+90), txt)
	DrawText(im, face, x+2, y-2, vwColor(k+60), txt)
	DrawText(im, face, x-2, y+2, vwColor(k+37), txt)
	DrawText(im, face, x+2, y+2, vwColor(k+80), txt)
}

// smooth applies PIL's SMOOTH kernel.
func smooth(im *image.NRGBA) *image.NRGBA {
	return dimg.Convolve3x3(im, [9]float64{1, 1, 1, 1, 5, 1, 1, 1, 1}, &dimg.ConvolveOptions{Normalize: true})
}

// saturate is PIL's ImageEnhance.Color: blend away from greyscale by k.
func saturate(im *image.NRGBA, k float64) *image.NRGBA {
	out := image.NewNRGBA(im.Rect)
	clamp := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v+0.5))) }
	for i := 0; i < len(im.Pix); i += 4 {
		r, g, b := float64(im.Pix[i]), float64(im.Pix[i+1]), float64(im.Pix[i+2])
		l := 0.299*r + 0.587*g + 0.114*b
		out.Pix[i] = clamp(l + k*(r-l))
		out.Pix[i+1] = clamp(l + k*(g-l))
		out.Pix[i+2] = clamp(l + k*(b-l))
		out.Pix[i+3] = im.Pix[i+3]
	}
	return out
}
