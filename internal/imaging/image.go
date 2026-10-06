// Package imaging downloads, decodes, transforms and encodes images for the
// image manipulation commands.
package imaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"time"

	"github.com/ericpauley/go-quantize/quantize"
	_ "golang.org/x/image/webp"

	"mayuri2000/internal/safehttp"
)

const (
	MaxDownload = 25 << 20
	// MaxPixels bounds the decoded size of a single frame.
	MaxPixels = 40_000_000
	// MaxFrames bounds the number of frames kept from animated inputs.
	MaxFrames = 200
)

var ErrNotImage = errors.New("that doesn't look like a supported image (png, jpeg, gif or webp) or MP4 video")

// httpClient refuses non-public addresses: image URLs come from users and
// the result is posted back, which would otherwise leak internal content.
var httpClient = safehttp.NewClient(30 * time.Second)

// Download fetches url, refusing bodies larger than MaxDownload.
func Download(ctx context.Context, url string) ([]byte, error) {
	req, err := safehttp.NewRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Mayuri2000 Discord bot)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading image: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxDownload {
		return nil, fmt.Errorf("image is larger than %d MiB", MaxDownload>>20)
	}
	return data, nil
}

// Sniff returns the image format of data based on its magic bytes, or "".
func Sniff(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return "jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		// ISO base media file: QuickTime's major brand means MOV, anything
		// else (isom, mp41, mp42, avc1, ...) is treated as MP4.
		if string(data[8:12]) == "qt  " {
			return "mov"
		}
		return "mp4"
	case bytes.HasPrefix(data, []byte("\x1a\x45\xdf\xa3")):
		return "webm" // EBML: WebM/Matroska
	}
	return ""
}

// IsVideo reports whether a sniffed format is a video container.
func IsVideo(format string) bool { return format == "mp4" || format == "mov" || format == "webm" }

// Image is a decoded still or animated image. Frames are fully composed, so
// each one can be transformed independently.
type Image struct {
	Frames []*image.NRGBA
	// Delays per frame in 100ths of a second (animated only).
	Delays []int
	Format string
	// Video is set when the image was decoded from a video; encoding then
	// produces a video (or a GIF for GIF-like sources) again.
	Video *VideoInfo
}

func (m *Image) Animated() bool      { return len(m.Frames) > 1 }
func (m *Image) First() *image.NRGBA { return m.Frames[0] }

// Decode decodes png, jpeg, webp and (animated) gif data.
func Decode(data []byte) (*Image, error) {
	format := Sniff(data)
	switch format {
	case "":
		return nil, ErrNotImage
	case "mp4":
		return nil, ErrVideoInput
	case "mov", "webm":
		return nil, ErrUnsupportedVideo
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	if cfg.Width*cfg.Height > MaxPixels {
		return nil, fmt.Errorf("image is too large (%dx%d)", cfg.Width, cfg.Height)
	}
	if format == "gif" {
		return decodeGIF(data)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	return &Image{Frames: []*image.NRGBA{ToNRGBA(img)}, Format: format}, nil
}

func decodeGIF(data []byte) (*Image, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	w, h := g.Config.Width, g.Config.Height
	if w == 0 || h == 0 {
		b := g.Image[0].Bounds()
		w, h = b.Max.X, b.Max.Y
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	out := &Image{Format: "gif"}
	for i, frame := range g.Image {
		if i >= MaxFrames {
			break
		}
		var backup *image.NRGBA
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			backup = cloneNRGBA(canvas)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		out.Frames = append(out.Frames, cloneNRGBA(canvas))
		delay := 10
		if i < len(g.Delay) && g.Delay[i] > 0 {
			delay = g.Delay[i]
		}
		out.Delays = append(out.Delays, delay)

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = backup
		}
	}
	return out, nil
}

// ToNRGBA converts any image into an NRGBA anchored at (0,0).
func ToNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out
}

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst
}

// Map applies fn to every frame.
func (m *Image) Map(fn func(*image.NRGBA) *image.NRGBA) *Image {
	out := &Image{Delays: m.Delays, Format: m.Format, Video: m.Video, Frames: make([]*image.NRGBA, len(m.Frames))}
	for i, f := range m.Frames {
		out.Frames[i] = fn(f)
	}
	return out
}

// Encode encodes the image as a GIF when animated, otherwise PNG. It returns
// the encoded data and the file extension. Video-derived images need
// EncodeContext.
func (m *Image) Encode() ([]byte, string, error) {
	return m.EncodeContext(context.Background())
}

// EncodeContext encodes video-derived images as MP4 (keeping their audio),
// GIF-like video and other animations as GIF, and stills as PNG.
func (m *Image) EncodeContext(ctx context.Context) ([]byte, string, error) {
	if m.Video != nil && !m.Video.GIFLike {
		b, err := encodeMP4(ctx, m)
		return b, "mp4", err
	}
	if m.Animated() {
		b, err := EncodeGIF(m.Frames, m.Delays)
		return b, "gif", err
	}
	b, err := EncodePNG(m.Frames[0])
	return b, "png", err
}

func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	// JPEG has no alpha; flatten onto black like PIL's convert("RGB").
	flat := image.NewRGBA(img.Bounds())
	draw.Draw(flat, flat.Rect, image.Black, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Rect, img, img.Bounds().Min, draw.Over)
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeGIF encodes frames as a looping GIF, quantising each frame to its own
// 256-colour palette (with one transparent entry when needed).
func EncodeGIF(frames []*image.NRGBA, delays []int) ([]byte, error) {
	g := &gif.GIF{LoopCount: 0}
	q := quantize.MedianCutQuantizer{}
	for i, f := range frames {
		transparent := hasTransparency(f)
		size := 256
		if transparent {
			size = 255
		}
		pal := q.Quantize(make(color.Palette, 0, size), opaque(f))
		if len(pal) == 0 {
			pal = color.Palette{color.Black}
		}
		if transparent {
			pal = append(color.Palette{color.NRGBA{}}, pal...)
		}
		p := image.NewPaletted(f.Rect, pal)
		palettize(p, f, transparent)
		g.Image = append(g.Image, p)
		delay := 10
		if i < len(delays) {
			delay = delays[i]
		}
		g.Delay = append(g.Delay, delay)
		g.Disposal = append(g.Disposal, gif.DisposalBackground)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hasTransparency(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 128 {
			return true
		}
	}
	return false
}

// opaque returns a view of img that the quantizer sees with full alpha, so
// transparent pixels don't pollute the palette.
func opaque(img *image.NRGBA) image.Image {
	out := image.NewRGBA(img.Rect)
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] < 128 {
			continue
		}
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255
	}
	return out
}

func palettize(dst *image.Paletted, src *image.NRGBA, transparent bool) {
	cache := make(map[uint32]uint8, 1024)
	w := src.Rect.Dx()
	for y := 0; y < src.Rect.Dy(); y++ {
		for x := 0; x < w; x++ {
			o := y*src.Stride + x*4
			r, g, b, a := src.Pix[o], src.Pix[o+1], src.Pix[o+2], src.Pix[o+3]
			var idx uint8
			if transparent && a < 128 {
				idx = 0
			} else {
				key := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
				var ok bool
				if idx, ok = cache[key]; !ok {
					idx = nearest(dst.Palette, r, g, b, transparent)
					cache[key] = idx
				}
			}
			dst.Pix[y*dst.Stride+x] = idx
		}
	}
}

func nearest(p color.Palette, r, g, b uint8, skipFirst bool) uint8 {
	best, bestDist := 0, int(^uint(0)>>1)
	start := 0
	if skipFirst {
		start = 1
	}
	for i := start; i < len(p); i++ {
		pr, pg, pb, _ := p[i].RGBA()
		dr, dg, db := int(r)-int(pr>>8), int(g)-int(pg>>8), int(b)-int(pb>>8)
		d := dr*dr + dg*dg + db*db
		if d < bestDist {
			best, bestDist = i, d
			if d == 0 {
				break
			}
		}
	}
	return uint8(best)
}
