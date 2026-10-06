package imaging

import (
	"bytes"
	"context"
	"image/jpeg"
	"math/rand/v2"
)

// Glitch corrupts an image. Stills are re-encoded as JPEG and have scan bytes
// overwritten (a port of Kareeeeem/jpglitch), then rendered through
// ImageMagick's lenient JPEG decoder; GIFs get their colour table scrambled
// as in NotSoBot's do_glitch. It returns the data and extension.
func Glitch(ctx context.Context, m *Magick, raw []byte, src *Image, amount, seed, iterations int) ([]byte, string, error) {
	// GIFs get their colour table scrambled. That has no MP4 equivalent,
	// so video (including Tenor/Klipy GIFs) glitches its first frame.
	if src.Animated() && src.Video == nil {
		return glitchGIF(raw), "gif", nil
	}
	base, err := EncodeJPEG(src.First(), 85)
	if err != nil {
		return nil, "", err
	}
	// Some corruptions are fatal to decoders; back off like jpglitch does.
	for it := iterations; it >= 1; it-- {
		out := jpglitch(base, amount, seed, it)
		if m != nil && m.Available {
			if png, err := m.Run(ctx, out, "jpeg:-", "-depth", "8", "png:-"); err == nil {
				return png, "png", nil
			} else if ctx.Err() != nil {
				return nil, "", err
			}
			continue
		}
		if _, err := jpeg.Decode(bytes.NewReader(out)); err == nil {
			return out, "jpg", nil
		}
	}
	return base, "jpg", nil
}

func jpglitch(data []byte, amount, seed, iterations int) []byte {
	out := append([]byte(nil), data...)
	header := 417
	for i := 0; i+3 < len(data); i++ {
		if data[i] == 0xFF && data[i+1] == 0xDA { // start of scan
			// Skip the SOS segment header itself so the scan stays parseable.
			header = i + 2 + (int(data[i+2])<<8 | int(data[i+3]))
			break
		}
	}
	maxIndex := len(data) - header - 4
	if maxIndex <= 0 {
		return out
	}
	a := float64(amount) / 100
	s := float64(seed) / 100
	for i := 0; i < iterations; i++ {
		pxMin := int(float64(maxIndex) / float64(iterations) * float64(i))
		pxMax := int(float64(maxIndex) / float64(iterations) * float64(i+1))
		pos := header + min(int(float64(pxMin)+float64(pxMax-pxMin)*s), maxIndex)
		// Keep JPEG byte stuffing intact: never touch a marker byte or the
		// byte following one, which strict decoders reject.
		for pos < len(out)-2 && (out[pos] == 0xFF || out[pos-1] == 0xFF) {
			pos++
		}
		out[pos] = byte(min(int(a*256), 0xFE))
	}
	return out
}

func glitchGIF(data []byte) []byte {
	out := append([]byte(nil), data...)
	end := -1
	for x := 13; x+1 < len(out); x++ {
		if out[x] == 0x21 && (out[x+1] == 0xFF || out[x+1] == 0xF9) {
			end = x
			break
		}
	}
	for x := 13; x < end; x++ {
		out[x] = byte(rand.IntN(256))
	}
	return out
}
