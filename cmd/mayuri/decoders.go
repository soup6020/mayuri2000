package main

import (
	"context"
	"log/slog"

	"mayuri2000/internal/config"
	"mayuri2000/internal/imaging"
	"mayuri2000/internal/sandbox"
)

// startDecoders checks the decoder sandbox and probes ImageMagick and
// ffmpeg. In required mode a sandbox that can't be fully applied disables
// both (fail closed); the rest of the bot keeps working.
func startDecoders(ctx context.Context, cfg *config.Config, log *slog.Logger,
	probe func(context.Context) (sandbox.Report, error),
) (*imaging.Magick, *imaging.FFmpeg) {
	sandbox.SetMode(cfg.DecoderSandbox)
	disabled := func() (*imaging.Magick, *imaging.FFmpeg) {
		return &imaging.Magick{Path: cfg.MagickPath}, &imaging.FFmpeg{Path: cfg.FFmpegPath}
	}

	r, err := probe(ctx)
	switch {
	case err != nil:
		log.Warn("decoder sandbox unavailable; image and video effects that need ImageMagick or ffmpeg are disabled (set DECODER_SANDBOX=best-effort to run them less isolated)",
			"err", err, "mode", cfg.DecoderSandbox)
		return disabled()
	case !r.Complete():
		log.Warn("decoder sandbox only partly applied (DECODER_SANDBOX=best-effort)", "problems", r.Problems,
			"landlock_abi", r.LandlockABI, "seccomp", r.Seccomp)
	default:
		log.Info("decoder sandbox active", "landlock_abi", r.LandlockABI, "seccomp", r.Seccomp, "rlimits", r.Rlimits)
	}

	magick, err := imaging.NewMagick(ctx, cfg.MagickPath)
	if err != nil {
		log.Warn("ImageMagick unavailable; magik, caption and watermark are disabled", "err", err)
	} else if !magick.LiquidRescale {
		log.Warn("ImageMagick lacks liquid rescale (liblqr) support; magik is disabled")
	}
	ff, err := imaging.NewFFmpeg(ctx, cfg.FFmpegPath)
	if err != nil {
		log.Warn("ffmpeg unavailable; MP4 video input is disabled", "err", err)
	}
	return magick, ff
}
