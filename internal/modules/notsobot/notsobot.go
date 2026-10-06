// Package notsobot ports the image manipulation commands of the NotSoBot cog.
package notsobot

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"strings"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/imagefind"
	"mayuri2000/internal/imaging"
)

const category = "Images"

type Module struct {
	finder *imagefind.Finder
	magick *imaging.Magick
	ff     *imaging.FFmpeg
	// effects maps command names to their effect, so tests can drive an
	// effect with fixture input without downloading.
	effects map[string]effect
}

func New(finder *imagefind.Finder, magick *imaging.Magick, ff *imaging.FFmpeg) *Module {
	return &Module{finder: finder, magick: magick, ff: ff, effects: map[string]effect{}}
}

// result is the output of an image effect.
type result struct {
	data    []byte
	ext     string
	content string
}

type effect func(c *cmd.Context, raw []byte, img *imaging.Image) (result, error)

// load finds, downloads and decodes the command's input image or video.
func (m *Module) load(c *cmd.Context) ([]byte, *imaging.Image, error) {
	srcs, err := m.finder.Find(c, 1)
	if err != nil {
		return nil, nil, err
	}
	return m.decodeSource(c, srcs[0])
}

// decodeSource downloads a source and decodes it as an image, or as a video
// through ffmpeg when its content is MP4.
func (m *Module) decodeSource(c *cmd.Context, src imagefind.Source) ([]byte, *imaging.Image, error) {
	videoOK := m.ff != nil && m.ff.Available
	if src.Kind != imagefind.Image && !videoOK {
		return nil, nil, cmd.Errorf("Video support needs ffmpeg, which isn't available right now.")
	}
	raw, err := imaging.Download(c, src.URL)
	if err != nil {
		return nil, nil, cmd.Errorf("I couldn't download that image: %s", err)
	}
	img, err := m.decodeBytes(c, raw, src.Kind)
	return raw, img, err
}

// decodeBytes decodes downloaded input as an image, or as a video through
// ffmpeg when its content is MP4.
func (m *Module) decodeBytes(c *cmd.Context, raw []byte, kind imagefind.Kind) (*imaging.Image, error) {
	videoOK := m.ff != nil && m.ff.Available
	var img *imaging.Image
	var err error
	switch imaging.Sniff(raw) {
	case "mp4":
		if !videoOK {
			return nil, cmd.Errorf("Video support needs ffmpeg, which isn't available right now.")
		}
		img, err = imaging.DecodeVideo(c, m.ff, raw, kind == imagefind.GIFVideo)
	default:
		img, err = imaging.Decode(raw)
	}
	switch {
	case err == nil:
		return img, nil
	case c.Err() != nil:
		return nil, cmd.Errorf("That took too long; try a shorter video or smaller image.")
	case errors.Is(err, imaging.ErrUnsupportedVideo), errors.Is(err, imaging.ErrNotImage):
		return nil, cmd.Errorf("%s", capitalize(err.Error()))
	case strings.HasPrefix(err.Error(), "ffmpeg failed"):
		return nil, cmd.Errorf("I couldn't read that video.")
	}
	return nil, cmd.Errorf("%s", capitalize(err.Error()))
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (m *Module) runEffect(name string, fn effect) func(*cmd.Context) error {
	return func(c *cmd.Context) error {
		raw, img, err := m.load(c)
		if err != nil {
			return err
		}
		res, err := fn(c, raw, img)
		if err != nil {
			if errors.Is(err, imaging.ErrMagickUnavailable) {
				return cmd.Errorf("This command needs ImageMagick, which isn't available right now.")
			}
			var ue *cmd.UserError
			if errors.As(err, &ue) {
				return err
			}
			if c.Err() != nil {
				return cmd.Errorf("That took too long; try a smaller image.")
			}
			return fmt.Errorf("%s: %w", name, err)
		}
		return c.ReplyFile(res.content, name+"."+res.ext, res.data)
	}
}

// still wraps a frame-wise transform into an effect.
func encoded(c *cmd.Context, img *imaging.Image) (result, error) {
	data, ext, err := img.EncodeContext(c)
	return result{data: data, ext: ext}, err
}

func (m *Module) imageCmd(name string, aliases []string, desc string, extra []cmd.Option, fn effect) *cmd.Command {
	m.effects[name] = fn
	return &cmd.Command{
		Name:        name,
		Aliases:     aliases,
		Category:    category,
		Description: desc,
		Options:     imagefind.Options(extra...),
		Heavy:       true,
		Run:         m.runEffect(name, fn),
	}
}

func intOpt(name, desc string, lo, hi int) cmd.Option {
	return cmd.Option{Name: name, Description: desc, Type: cmd.Int, Min: cmd.Ptr(lo), Max: cmd.Ptr(hi)}
}

func (m *Module) Commands() []*cmd.Command {
	merge := m.imageCmd("merge", nil, "Merge several images together",
		[]cmd.Option{
			{Name: "image2", Description: "Second image to merge", Type: cmd.Attachment},
			{Name: "vertical", Description: "Stack vertically (default true)", Type: cmd.Bool},
		}, nil)
	merge.Run = m.merge
	return []*cmd.Command{
		merge,
		m.imageCmd("magik", []string{"imagemagic", "imagemagick", "magic", "magick", "cas", "liquid"},
			"Apply content-aware scaling magik to an image",
			[]cmd.Option{intOpt("scale", "Distortion strength (default 2)", 1, 10)},
			m.magik),
		m.imageCmd("gmagik", nil, "Magik every frame of a GIF, or animate a still image",
			nil, m.gmagik),
		m.imageCmd("petimg", []string{"pet", "petpet"}, "Pet an image or someone's avatar", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				data, err := imaging.Petpet(img)
				return result{data: data, ext: "gif"}, err
			}),
		m.imageCmd("caption", nil, "Add a caption to an image",
			[]cmd.Option{
				{Name: "text", Description: "Caption text", Type: cmd.String, MaxLength: cmd.Ptr(500)},
				{Name: "color", Description: "Text colour name or hex (default white)", Type: cmd.String},
				intOpt("size", "Font size (default 40)", 5, 300),
				intOpt("x", "Horizontal position, 0-100% from the left", 0, 100),
				intOpt("y", "Vertical position, 0-100% from the top", 0, 100),
			}, m.caption),
		m.imageCmd("triggered", nil, "Generate a TRIGGERED GIF", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				data, err := imaging.Triggered(img)
				return result{data: data, ext: "gif"}, err
			}),
		m.imageCmd("jpeg", []string{"needsmorejpeg", "jpegify", "magik2"}, "Add more JPEG to an image",
			[]cmd.Option{intOpt("quality", "JPEG quality from 1 (worst) to 10 (default 1)", 1, 10)},
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				data, err := imaging.EncodeJPEG(img.First(), c.IntOr("quality", 1))
				return result{data: data, ext: "jpg"}, err
			}),
		m.imageCmd("vw", []string{"vaporwave", "vape", "vapewave"}, "Add vaporwave flavours to an image",
			[]cmd.Option{{Name: "text", Description: "Text to add (default \"vapor wave\")", Type: cmd.String, Rest: true, MaxLength: cmd.Ptr(100)}},
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				out := imaging.Vaporwave(imaging.FitWithin(img, 1500).First(), c.StringOr("text", "vapor wave"))
				data, err := imaging.EncodePNG(out)
				return result{data: data, ext: "png"}, err
			}),
		m.imageCmd("watermark", []string{"wm"}, "Add a watermark (brazzers by default) to an image",
			[]cmd.Option{
				{Name: "mark", Description: "Watermark image", Type: cmd.Attachment},
				{Name: "mark_url", Description: "Watermark image URL", Type: cmd.String, Accept: imagefind.LooksLikeImageRef},
				intOpt("x", "Horizontal position, 0-100% from the left", 0, 100),
				intOpt("y", "Vertical position, 0-100% from the top", 0, 100),
				intOpt("transparency", "Watermark transparency 0-100%", 0, 100),
			}, m.watermark),
		m.imageCmd("glitch", []string{"jpglitch"}, "Glitch an image or GIF",
			[]cmd.Option{
				intOpt("iterations", "Number of corruptions (random by default)", 1, 100),
				intOpt("amount", "Corruption value 0-99 (random by default)", 0, 99),
				intOpt("seed", "Corruption position 0-99 (random by default)", 0, 99),
			}, func(c *cmd.Context, raw []byte, img *imaging.Image) (result, error) {
				it := c.IntOr("iterations", rand.IntN(30)+1)
				amount := c.IntOr("amount", rand.IntN(20)+1)
				seed := c.IntOr("seed", rand.IntN(20)+1)
				data, ext, err := imaging.Glitch(c, m.magick, raw, img, amount, seed, it)
				content := fmt.Sprintf("Iterations: `%d` | Amount: `%d` | Seed: `%d`", it, amount, seed)
				if img.Animated() && img.Video == nil {
					content = "" // GIF glitches don't use these parameters
				}
				return result{data: data, ext: ext, content: content}, err
			}),
		m.imageCmd("pixelate", []string{"pixel"}, "Pixelate an image",
			[]cmd.Option{intOpt("pixels", "Block size in pixels (default 9)", 2, 100)},
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.Pixelate(imaging.FitWithin(img, 2048), c.IntOr("pixels", 9)))
			}),
		m.mirrorCmd("waaw", []string{"magik3", "mirror"}, "Mirror the right half of an image onto the left", imaging.Waaw),
		m.mirrorCmd("haah", []string{"magik4", "mirror2"}, "Mirror the left half of an image onto the right", imaging.Haah),
		m.mirrorCmd("woow", []string{"magik5", "mirror3"}, "Mirror the top half of an image onto the bottom", imaging.Woow),
		m.mirrorCmd("hooh", []string{"magik6", "mirror4"}, "Mirror the bottom half of an image onto the top", imaging.Hooh),
		m.imageCmd("flipimg", []string{"flip"}, "Flip an image upside down", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.Flip(img))
			}),
		m.imageCmd("flop", nil, "Mirror an image horizontally", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.Flop(img))
			}),
		m.imageCmd("invert", []string{"inverse", "negate"}, "Invert the colours of an image", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.Invert(img))
			}),
		m.rotateCmd(),
		m.imageCmd("deepfry", []string{"fry", "df"}, "Deep fry an image",
			[]cmd.Option{
				{Name: "scale", Description: "Intensity from 1 to 20 (default 10)", Type: cmd.Float, Min: cmd.Ptr(1), Max: cmd.Ptr(20)},
				{Name: "notransparency", Description: "Don't keep transparency", Type: cmd.Bool},
			}, func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				fried, err := imaging.Deepfry(imaging.FitWithin(img, 1024), c.FloatOr("scale", 10), !c.Bool("notransparency"))
				if err != nil {
					return result{}, err
				}
				return encoded(c, fried)
			}),
		m.imageCmd("solarize", nil, "Apply a solarize tint to an image", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.Solarize(imaging.FitWithin(img, 2048)))
			}),
		m.imageCmd("shake", nil, "Shake an image",
			[]cmd.Option{
				{Name: "horizontal", Description: "Horizontal magnitude, % of width (default 4)", Type: cmd.Float, Min: cmd.Ptr(0), Max: cmd.Ptr(25)},
				{Name: "vertical", Description: "Vertical magnitude, % of height (default 4)", Type: cmd.Float, Min: cmd.Ptr(0), Max: cmd.Ptr(25)},
				{Name: "randomize", Description: "Shake randomly instead of in a pattern", Type: cmd.Bool},
			}, func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				shaken := imaging.Shake(imaging.FitWithin(img, 512), imaging.ShakeOpts{
					Horizontal: c.Float("horizontal"), Vertical: c.Float("vertical"), Randomize: c.Bool("randomize"),
				})
				return encoded(c, shaken)
			}),
		m.imageCmd("swapcolors", nil, "Shuffle each pixel's colour channels", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.SwapColors(imaging.FitWithin(img, 1024)))
			}),
		m.imageCmd("swappixels", nil, "Shuffle every pixel of an image around", nil,
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				return encoded(c, imaging.SwapPixels(imaging.FitWithin(img, 1024)))
			}),
		m.imageCmd("swaprgba", nil, "Rearrange an image's RGBA channels",
			[]cmd.Option{
				{Name: "channels", Description: "New channel order using r, g, b, a, 0 or 1, e.g. bgr, rrba, 0g0a", Type: cmd.String, Required: true, MaxLength: cmd.Ptr(4)},
			}, func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				channels, err := imaging.ParseChannels(c.String("channels"))
				if err != nil {
					return result{}, cmd.Errorf("%s", capitalize(err.Error()))
				}
				return encoded(c, imaging.SwapRGBA(imaging.FitWithin(img, 2048), channels))
			}),
		m.imageCmd("wall", nil, "Tile an image across a wall", nil,
			func(c *cmd.Context, raw []byte, img *imaging.Image) (result, error) {
				if img.Video != nil {
					return m.videoFrames(c, img, func(frame []byte, still *imaging.Image) ([]byte, error) {
						data, _, err := m.magick.Wall(c, frame, still)
						return data, err
					})
				}
				if len(img.Frames) > 100 {
					return result{}, cmd.Errorf("That GIF has too many frames (%d, max 100).", len(img.Frames))
				}
				data, ext, err := m.magick.Wall(c, raw, img)
				return result{data: data, ext: ext}, err
			}),
		m.imageCmd("watercolor", []string{"watercolour"}, "Apply a watercolor effect to an image",
			[]cmd.Option{
				{Name: "contrast", Description: "Contrast from -100 to 100 (default 0)", Type: cmd.Float, Min: cmd.Ptr(-100), Max: cmd.Ptr(100)},
				{Name: "edge", Description: "Edge gain from 0 to 10 (default 2)", Type: cmd.Float, Min: cmd.Ptr(0), Max: cmd.Ptr(10)},
				{Name: "mixing", Description: "Edge mixing from 0 to 100 (default 33)", Type: cmd.Int, Min: cmd.Ptr(0), Max: cmd.Ptr(100)},
				{Name: "smoothing", Description: "Texture smoothing from 0 to 10 (default 0)", Type: cmd.Int, Min: cmd.Ptr(0), Max: cmd.Ptr(10)},
			}, func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				// Videos are already bounded by the video limits.
				if img.Video == nil && len(img.Frames) > 50 {
					return result{}, cmd.Errorf("That GIF has too many frames for watercolor (%d, max 50).", len(img.Frames))
				}
				data, ext, err := m.magick.Watercolor(c, imaging.FitWithin(img, 800), imaging.WatercolorOpts{
					Contrast: c.FloatOr("contrast", 0), Edge: c.FloatOr("edge", 2),
					Mixing: c.IntOr("mixing", 33), Smoothing: c.IntOr("smoothing", 0),
				})
				return result{data: data, ext: ext}, err
			}),
		m.imageCmd("iascii", []string{"gascii"}, "Turn an image or GIF into ASCII art",
			[]cmd.Option{intOpt("columns", "Width in characters (default 80)", 10, 200)},
			func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
				data, ext, err := imaging.ASCIIArt(c, img, c.IntOr("columns", 80))
				return result{data: data, ext: ext}, err
			}),
		{
			Name:        "ascii",
			Aliases:     []string{"expand"},
			Category:    category,
			Description: "Render text as big ASCII art",
			Options:     []cmd.Option{{Name: "text", Description: "Text to render", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(200)}},
			Heavy:       true,
			Run:         asciiText,
		},
		{
			Name:        "rip",
			Category:    category,
			Description: "Generate a tombstone",
			Options: []cmd.Option{
				{Name: "name", Description: "Who (or what) died; defaults to you", Type: cmd.String, MaxLength: cmd.Ptr(60)},
				{Name: "text", Description: "Epitaph", Type: cmd.String, Rest: true, MaxLength: cmd.Ptr(120)},
			},
			Heavy: true,
			Run:   rip,
		},
		{
			Name:        "minecraftachievement",
			Aliases:     []string{"achievement"},
			Category:    category,
			Description: "Generate a Minecraft achievement",
			Options:     []cmd.Option{{Name: "text", Description: "Achievement text", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(100)}},
			Heavy:       true,
			Run: func(c *cmd.Context) error {
				data, err := imaging.Achievement(c.String("text"))
				if err != nil {
					return err
				}
				return c.ReplyFile("", "achievement.png", data)
			},
		},
		{
			Name:        "aesthetics",
			Aliases:     []string{"aes"},
			Category:    "Text",
			Description: "Ｆｕｌｌ ｗｉｄｔｈ text",
			Options:     []cmd.Option{{Name: "text", Description: "Text to convert", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(1000)}},
			Run: func(c *cmd.Context) error {
				return c.Reply(Aesthetics(c.String("text")))
			},
		},
		{
			Name:        "emojify",
			Category:    "Text",
			Description: "Replace letters and numbers with emoji",
			Options:     []cmd.Option{{Name: "text", Description: "Text to convert", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(200)}},
			Run: func(c *cmd.Context) error {
				out := Emojify(c.String("text"))
				if len([]rune(out)) > 2000 {
					return cmd.Errorf("That's too long to emojify.")
				}
				return c.Reply(out)
			},
		},
	}
}

func (m *Module) mirrorCmd(name string, aliases []string, desc string, mode imaging.MirrorMode) *cmd.Command {
	return m.imageCmd(name, aliases, desc, nil, func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
		return encoded(c, imaging.Mirror(imaging.FitWithin(img, 2048), mode))
	})
}

func (m *Module) rotateCmd() *cmd.Command {
	c := m.imageCmd("rotate", nil, "Rotate an image counter-clockwise",
		[]cmd.Option{intOpt("degrees", "Degrees to rotate (default 90)", -360, 360)},
		func(c *cmd.Context, _ []byte, img *imaging.Image) (result, error) {
			deg := c.IntOr("degrees", 90)
			res, err := encoded(c, imaging.Rotate(imaging.FitWithin(img, 2048), float64(deg)))
			res.content = fmt.Sprintf("Rotated: `%d°`", deg)
			return res, err
		})
	// NotSoBot took degrees before the image: `rotate 45 <url>`.
	opts := c.Options
	c.Options = append([]cmd.Option{opts[3]}, opts[:3]...)
	return c
}

func (m *Module) magik(c *cmd.Context, raw []byte, img *imaging.Image) (result, error) {
	if !m.magick.LiquidRescale {
		return result{}, cmd.Errorf("Magik needs ImageMagick with liquid rescale support.")
	}
	if img.Animated() || img.Video != nil {
		return m.gmagik(c, raw, img)
	}
	data, err := m.magick.Magik(c, raw, c.IntOr("scale", 2))
	return result{data: data, ext: "png"}, err
}

func (m *Module) gmagik(c *cmd.Context, raw []byte, img *imaging.Image) (result, error) {
	if !m.magick.LiquidRescale {
		return result{}, cmd.Errorf("Magik needs ImageMagick with liquid rescale support.")
	}
	if img.Video != nil {
		return m.videoFrames(c, img, func(frame []byte, _ *imaging.Image) ([]byte, error) {
			return m.magick.LiquidFrame(c, frame)
		})
	}
	if len(img.Frames) > 100 {
		return result{}, cmd.Errorf("That GIF has too many frames (%d, max 100).", len(img.Frames))
	}
	data, err := m.magick.GMagik(c, raw, img)
	return result{data: data, ext: "gif"}, err
}

// videoFrames runs an ImageMagick operation on each video frame (as PNG)
// and reassembles the result, keeping the video's frame rate and audio.
// The bounded video limits (≤150 frames at ≤512 px) keep this tractable.
func (m *Module) videoFrames(c *cmd.Context, img *imaging.Image, op func(frame []byte, still *imaging.Image) ([]byte, error)) (result, error) {
	out := &imaging.Image{Delays: img.Delays, Format: img.Format, Video: img.Video}
	for _, f := range img.Frames {
		png, err := imaging.EncodePNG(f)
		if err != nil {
			return result{}, err
		}
		res, err := op(png, &imaging.Image{Frames: []*image.NRGBA{f}})
		if err != nil {
			return result{}, err
		}
		dec, err := imaging.Decode(res)
		if err != nil {
			return result{}, err
		}
		out.Frames = append(out.Frames, dec.First())
	}
	return encoded(c, out)
}

func (m *Module) caption(c *cmd.Context, raw []byte, img *imaging.Image) (result, error) {
	textColor, ok := imaging.ParseColor(c.StringOr("color", "white"))
	if !ok {
		return result{}, cmd.Errorf("That is not a valid colour.")
	}
	opts := imaging.CaptionOpts{
		Text:  c.StringOr("text", "Caption"),
		Color: textColor,
		Size:  c.IntOr("size", 40),
		X:     c.IntOr("x", 0),
		Y:     c.IntOr("y", 0),
	}
	if img.Video != nil {
		return m.videoFrames(c, img, func(frame []byte, still *imaging.Image) ([]byte, error) {
			data, _, err := m.magick.Caption(c, frame, still, opts)
			return data, err
		})
	}
	data, ext, err := m.magick.Caption(c, raw, img, opts)
	return result{data: data, ext: ext}, err
}

func (m *Module) watermark(c *cmd.Context, raw []byte, img *imaging.Image) (result, error) {
	var mark []byte
	markURL := ""
	if a, ok := c.OptAttachment("mark"); ok {
		markURL = a.URL
	} else if u, ok := c.OptString("mark_url"); ok {
		markURL = u
	}
	if markURL != "" {
		var err error
		if mark, err = imaging.Download(c, markURL); err != nil {
			return result{}, cmd.Errorf("I couldn't download the watermark: %s", err)
		}
		if f := imaging.Sniff(mark); f == "" || imaging.IsVideo(f) {
			return result{}, cmd.Errorf("The watermark must be an image (png, jpeg, gif or webp).")
		}
	}
	opts := imaging.WatermarkOpts{
		Mark:         mark,
		X:            c.IntOr("x", 0),
		Y:            c.IntOr("y", 0),
		Transparency: c.IntOr("transparency", 0),
	}
	if img.Video != nil {
		return m.videoFrames(c, img, func(frame []byte, still *imaging.Image) ([]byte, error) {
			data, _, err := m.magick.Watermark(c, frame, still, opts)
			return data, err
		})
	}
	data, ext, err := m.magick.Watermark(c, raw, img, opts)
	return result{data: data, ext: ext}, err
}

func (m *Module) merge(c *cmd.Context) error {
	srcs, err := m.finder.Find(c, 10)
	if err != nil {
		return err
	}
	if a, ok := c.OptAttachment("image2"); ok {
		srcs = append(srcs, imagefind.Source{URL: a.URL})
	}
	if len(srcs) < 2 {
		return cmd.Errorf("You need to supply at least 2 images.")
	}
	var imgs []*imaging.Image
	for i, src := range srcs {
		_, img, err := m.decodeSource(c, src)
		if err != nil {
			return cmd.Errorf("Image %d: %s", i+1, err)
		}
		imgs = append(imgs, img) // videos contribute their first frame
	}
	vertical := true
	if v, ok := c.OptBool("vertical"); ok {
		vertical = v
	}
	data, err := imaging.EncodePNG(imaging.Merge(imgs, vertical))
	if err != nil {
		return err
	}
	return c.ReplyFile("", "merge.png", data)
}

func asciiText(c *cmd.Context) error {
	text := c.String("text")
	if text == "donger" || text == "dong" {
		text = "8====D"
	}
	art, err := imaging.Figlet(text)
	if err != nil {
		return cmd.Errorf(":no_entry: go away with your invalid characters.")
	}
	data, err := imaging.EncodePNG(imaging.RenderText(art, color.NRGBA{G: 255, A: 255}, color.Black, 15))
	if err != nil {
		return err
	}
	content := ""
	if len(art) <= 600 {
		content = "```fix\n" + art + "```"
	}
	return c.ReplyFile(content, "ascii.png", data)
}

func rip(c *cmd.Context) error {
	name := c.StringOr("name", c.Author.EffectiveName())
	if id, ok := cmd.ParseUserID(name); ok {
		if u, err := c.Client.Rest.GetUser(id); err == nil {
			name = u.EffectiveName()
		}
	}
	text := name + "'s\nHopes and Dreams"
	if t, ok := c.OptString("text"); ok {
		text = name + "\n" + t
	}
	data, err := imaging.RIP(text)
	if err != nil {
		return err
	}
	return c.ReplyFile("", "rip.jpg", data)
}

// Menus returns the context menu commands for image effects.
func (m *Module) Menus(cmds []*cmd.Command) []*cmd.Menu {
	byName := map[string]*cmd.Command{}
	for _, c := range cmds {
		byName[c.Name] = c
	}
	menu := func(label, name string, kind cmd.MenuKind) *cmd.Menu {
		return &cmd.Menu{Name: label, Kind: kind, Heavy: true, Run: byName[name].Run}
	}
	return []*cmd.Menu{
		menu("Magik", "magik", cmd.MenuMessage),
		menu("JPEG", "jpeg", cmd.MenuMessage),
		menu("Glitch", "glitch", cmd.MenuMessage),
		menu("GMagik", "gmagik", cmd.MenuMessage),
		menu("Invert", "invert", cmd.MenuMessage),
		menu("Triggered", "triggered", cmd.MenuUser),
		menu("Pet", "petimg", cmd.MenuUser),
	}
}
