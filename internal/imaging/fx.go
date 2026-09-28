package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand/v2"
	"strings"

	dimg "github.com/disintegration/imaging"
)

// Solarize inverts every channel value above the midpoint, like
// ImageMagick's -solarize 50%.
func Solarize(m *Image) *Image {
	return m.Map(func(f *image.NRGBA) *image.NRGBA {
		out := cloneNRGBA(f)
		for i := 0; i < len(out.Pix); i += 4 {
			for c := 0; c < 3; c++ {
				if v := out.Pix[i+c]; v > 127 {
					out.Pix[i+c] = 255 - v
				}
			}
		}
		return out
	})
}

// Deepfry over-saturates, over-sharpens, adds noise and crushes the image
// through low quality JPEG. scale (1-20, default 10) controls intensity.
// With keepAlpha the original transparency is restored afterwards.
func Deepfry(m *Image, scale float64, keepAlpha bool) (*Image, error) {
	scale = math.Max(1, math.Min(20, scale))
	var ferr error
	rng := rand.New(rand.NewPCG(1, 2))
	out := m.Map(func(f *image.NRGBA) *image.NRGBA {
		if ferr != nil {
			return f
		}
		img := dimg.AdjustContrast(f, 5*scale)
		img = saturate(img, 1+0.35*scale)
		img = dimg.Sharpen(img, 0.4*scale)
		// Warm, blown-out tint.
		img = dimg.AdjustFunc(img, func(c color.NRGBA) color.NRGBA {
			c.R = clamp8(float64(c.R)*1.1 + 4*scale)
			c.B = clamp8(float64(c.B) * (1 - 0.02*scale))
			return c
		})
		// Grain.
		amp := 2.5 * scale
		for i := 0; i < len(img.Pix); i += 4 {
			n := (rng.Float64()*2 - 1) * amp
			for c := 0; c < 3; c++ {
				img.Pix[i+c] = clamp8(float64(img.Pix[i+c]) + n)
			}
		}
		// Crush through JPEG a few times at shrinking size for chunkier artifacts.
		quality := max(1, 25-int(scale))
		w, h := img.Rect.Dx(), img.Rect.Dy()
		shrink := 1 - 0.02*scale
		small := dimg.Resize(img, max(1, int(float64(w)*shrink)), max(1, int(float64(h)*shrink)), dimg.Linear)
		fried, err := jpegRoundTrip(small, quality, 2)
		if err != nil {
			ferr = err
			return f
		}
		fried = dimg.Resize(fried, w, h, dimg.Linear)
		if keepAlpha {
			for i := 3; i < len(fried.Pix); i += 4 {
				fried.Pix[i] = f.Pix[i]
			}
		}
		return fried
	})
	return out, ferr
}

func clamp8(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

func jpegRoundTrip(img *image.NRGBA, quality, times int) (*image.NRGBA, error) {
	cur := image.Image(img)
	for range times {
		data, err := EncodeJPEG(cur, quality)
		if err != nil {
			return nil, err
		}
		if cur, err = jpeg.Decode(bytes.NewReader(data)); err != nil {
			return nil, err
		}
	}
	return ToNRGBA(cur), nil
}

// ShakeOpts controls Shake. Magnitudes are percentages of the image size.
type ShakeOpts struct {
	Horizontal, Vertical float64
	Randomize            bool
}

// Shake makes an animation of the image jittering around. Stills become a
// 12 frame loop; animations are shaken frame by frame.
func Shake(m *Image, o ShakeOpts) *Image {
	if o.Horizontal == 0 && o.Vertical == 0 {
		o.Horizontal, o.Vertical = 4, 4
	}
	o.Horizontal = math.Max(0, math.Min(25, o.Horizontal))
	o.Vertical = math.Max(0, math.Min(25, o.Vertical))
	frames, delays := m.Frames, m.Delays
	if !m.Animated() {
		frames = make([]*image.NRGBA, 12)
		delays = make([]int, 12)
		for i := range frames {
			frames[i], delays[i] = m.First(), 4
		}
	}
	w, h := frames[0].Rect.Dx(), frames[0].Rect.Dy()
	dx, dy := int(float64(w)*o.Horizontal/100), int(float64(h)*o.Vertical/100)
	rng := rand.New(rand.NewPCG(3, 4))
	out := &Image{Format: "gif", Delays: delays}
	for i, f := range frames {
		// Scale up so the moving crop window never shows empty borders.
		big := dimg.Resize(f, w+2*dx, h+2*dy, dimg.Lanczos)
		var ox, oy float64
		if o.Randomize {
			ox, oy = rng.Float64()*2-1, rng.Float64()*2-1
		} else {
			// Alternate between opposite corners for a classic jitter.
			t := float64(i) * math.Pi * 0.9
			ox, oy = math.Cos(t), math.Sin(t*1.7)
		}
		x, y := dx+int(ox*float64(dx)), dy+int(oy*float64(dy))
		out.Frames = append(out.Frames, dimg.Crop(big, image.Rect(x, y, x+w, y+h)))
	}
	return out
}

// SwapPixels shuffles every pixel position. The same permutation is used on
// every frame so animations don't flicker.
func SwapPixels(m *Image) *Image {
	f := m.First()
	n := f.Rect.Dx() * f.Rect.Dy()
	perm := rand.New(rand.NewPCG(5, 6)).Perm(n)
	return m.Map(func(f *image.NRGBA) *image.NRGBA {
		out := image.NewNRGBA(f.Rect)
		for i, j := range perm {
			copy(out.Pix[i*4:i*4+4], f.Pix[j*4:j*4+4])
		}
		return out
	})
}

// SwapColors shuffles the red, green and blue channels of each pixel
// independently, keeping alpha. The shuffle is consistent across frames.
func SwapColors(m *Image) *Image {
	perms := [6][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	f := m.First()
	n := f.Rect.Dx() * f.Rect.Dy()
	rng := rand.New(rand.NewPCG(7, 8))
	choice := make([]uint8, n)
	for i := range choice {
		choice[i] = uint8(rng.IntN(len(perms)))
	}
	return m.Map(func(f *image.NRGBA) *image.NRGBA {
		out := cloneNRGBA(f)
		for i := 0; i < n; i++ {
			p := perms[choice[i]]
			o := i * 4
			r, g, b := f.Pix[o], f.Pix[o+1], f.Pix[o+2]
			src := [3]uint8{r, g, b}
			out.Pix[o], out.Pix[o+1], out.Pix[o+2] = src[p[0]], src[p[1]], src[p[2]]
		}
		return out
	})
}

// ParseChannels validates a swaprgba spec: 3 or 4 characters, each one of
// r, g, b, a (copy that channel), 0 (empty) or 1 (full). A 3 character spec
// keeps the alpha channel.
func ParseChannels(spec string) ([4]int, error) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if len(spec) == 3 {
		spec += "a"
	}
	var out [4]int
	if len(spec) != 4 {
		return out, fmt.Errorf("channels must be 3 or 4 characters, like `bgr` or `rrba`")
	}
	for i, c := range spec {
		switch c {
		case 'r':
			out[i] = 0
		case 'g':
			out[i] = 1
		case 'b':
			out[i] = 2
		case 'a':
			out[i] = 3
		case '0':
			out[i] = -1
		case '1':
			out[i] = -2
		default:
			return out, fmt.Errorf("`%c` isn't a channel; use r, g, b, a, 0 or 1", c)
		}
	}
	return out, nil
}

// SwapRGBA rebuilds each pixel's channels from the given sources (see
// ParseChannels).
func SwapRGBA(m *Image, channels [4]int) *Image {
	return m.Map(func(f *image.NRGBA) *image.NRGBA {
		out := image.NewNRGBA(f.Rect)
		for o := 0; o < len(f.Pix); o += 4 {
			for c, src := range channels {
				switch src {
				case -1:
					out.Pix[o+c] = 0
				case -2:
					out.Pix[o+c] = 255
				default:
					out.Pix[o+c] = f.Pix[o+src]
				}
			}
		}
		return out
	})
}
