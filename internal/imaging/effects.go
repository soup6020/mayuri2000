package imaging

import (
	"image"
	"image/color"
	"image/draw"

	dimg "github.com/disintegration/imaging"
)

// Flip flips vertically (top to bottom), like PIL's ImageOps.flip.
func Flip(m *Image) *Image { return m.Map(func(f *image.NRGBA) *image.NRGBA { return dimg.FlipV(f) }) }

// Flop mirrors horizontally, like PIL's ImageOps.mirror.
func Flop(m *Image) *Image { return m.Map(func(f *image.NRGBA) *image.NRGBA { return dimg.FlipH(f) }) }

// Invert inverts colours, keeping alpha.
func Invert(m *Image) *Image {
	return m.Map(func(f *image.NRGBA) *image.NRGBA { return dimg.Invert(f) })
}

// Rotate rotates counter-clockwise by degrees, expanding the canvas.
func Rotate(m *Image, degrees float64) *Image {
	return m.Map(func(f *image.NRGBA) *image.NRGBA { return dimg.Rotate(f, degrees, color.Transparent) })
}

// Pixelate downsamples into blocks of size px and draws a black grid, like
// NotSoBot's make_pixel.
func Pixelate(m *Image, px int) *Image {
	if px < 2 {
		px = 2
	}
	return m.Map(func(f *image.NRGBA) *image.NRGBA {
		w, h := f.Rect.Dx(), f.Rect.Dy()
		sw, sh := max(1, w/px), max(1, h/px)
		small := dimg.Resize(f, sw, sh, dimg.NearestNeighbor)
		out := dimg.Resize(small, sw*px, sh*px, dimg.NearestNeighbor)
		black := color.NRGBA{A: 255}
		for x := 0; x < out.Rect.Dx(); x += px {
			for y := 0; y < out.Rect.Dy(); y += px {
				for r := 0; r < px; r++ {
					out.SetNRGBA(x+r, y, black)
					out.SetNRGBA(x, y+r, black)
				}
			}
		}
		return out
	})
}

// MirrorMode selects one of NotSoBot's mirror effects.
type MirrorMode int

const (
	// Waaw mirrors the right half onto the left.
	Waaw MirrorMode = iota
	// Haah mirrors the left half onto the right.
	Haah
	// Woow mirrors the top half onto the bottom.
	Woow
	// Hooh mirrors the bottom half onto the top.
	Hooh
)

func Mirror(m *Image, mode MirrorMode) *Image {
	return m.Map(func(f *image.NRGBA) *image.NRGBA {
		w, h := f.Rect.Dx(), f.Rect.Dy()
		hw, hh := max(1, w/2), max(1, h/2)
		switch mode {
		case Waaw:
			half := dimg.Crop(f, image.Rect(w-hw, 0, w, h))
			return hstack(dimg.FlipH(half), half)
		case Haah:
			half := dimg.Crop(f, image.Rect(0, 0, hw, h))
			return hstack(half, dimg.FlipH(half))
		case Woow:
			half := dimg.Crop(f, image.Rect(0, 0, w, hh))
			return vstack(half, dimg.FlipV(half))
		default:
			half := dimg.Crop(f, image.Rect(0, h-hh, w, h))
			return vstack(dimg.FlipV(half), half)
		}
	})
}

func hstack(imgs ...*image.NRGBA) *image.NRGBA {
	w, h := 0, 0
	for _, i := range imgs {
		w += i.Rect.Dx()
		h = max(h, i.Rect.Dy())
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	x := 0
	for _, i := range imgs {
		draw.Draw(out, image.Rect(x, 0, x+i.Rect.Dx(), i.Rect.Dy()), i, i.Rect.Min, draw.Src)
		x += i.Rect.Dx()
	}
	return out
}

func vstack(imgs ...*image.NRGBA) *image.NRGBA {
	w, h := 0, 0
	for _, i := range imgs {
		h += i.Rect.Dy()
		w = max(w, i.Rect.Dx())
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	y := 0
	for _, i := range imgs {
		draw.Draw(out, image.Rect(0, y, i.Rect.Dx(), y+i.Rect.Dy()), i, i.Rect.Min, draw.Src)
		y += i.Rect.Dy()
	}
	return out
}

// Merge stacks the first frame of each image, scaling them to a common width
// (vertical) or height (horizontal) so nothing is distorted.
func Merge(imgs []*Image, vertical bool) *image.NRGBA {
	frames := make([]*image.NRGBA, len(imgs))
	target := 0
	for i, m := range imgs {
		f := m.First()
		frames[i] = f
		size := f.Rect.Dy()
		if vertical {
			size = f.Rect.Dx()
		}
		if target == 0 || size < target {
			target = size
		}
	}
	target = min(target, 2048)
	for i, f := range frames {
		if vertical {
			frames[i] = dimg.Resize(f, target, 0, dimg.Lanczos)
		} else {
			frames[i] = dimg.Resize(f, 0, target, dimg.Lanczos)
		}
	}
	if vertical {
		return vstack(frames...)
	}
	return hstack(frames...)
}

// FitWithin shrinks every frame so that neither side exceeds size.
func FitWithin(m *Image, size int) *Image {
	f := m.First()
	if f.Rect.Dx() <= size && f.Rect.Dy() <= size {
		return m
	}
	return m.Map(func(f *image.NRGBA) *image.NRGBA { return dimg.Fit(f, size, size, dimg.Lanczos) })
}
