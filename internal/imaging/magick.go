package imaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mayuri2000/internal/sandbox"
)

// Magick runs the ImageMagick CLI.
type Magick struct {
	Path string
	// LiquidRescale reports whether ImageMagick was built with liblqr.
	LiquidRescale bool
	Available     bool
}

// NewMagick probes the magick binary and its features.
func NewMagick(ctx context.Context, path string) (*Magick, error) {
	m := &Magick{Path: path}
	// The probe runs sandboxed like every job, so a sandbox too strict for
	// ImageMagick shows up here rather than on a user's command.
	out, err := runSandboxed(ctx, nil, nil, path, "-version")
	if err != nil {
		return m, fmt.Errorf("running %s -version: %w", path, err)
	}
	m.Available = true
	m.LiquidRescale = strings.Contains(string(out), "lqr")
	return m, nil
}

var ErrMagickUnavailable = errors.New("ImageMagick is not available")

// Run feeds input to magick on stdin and returns stdout. args must contain
// the input spec and the output spec (e.g. "png:-"). The input coder is
// always forced from the sniffed format so untrusted data can never select
// a dangerous coder (MVG, MSL, SVG, ...).
func (m *Magick) Run(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	return m.RunWith(ctx, input, nil, args...)
}

// RunWith is Run with extra input files (e.g. a watermark), written into the
// job's directory; args refer to them by bare name, as the job directory is
// magick's working directory.
func (m *Magick) RunWith(ctx context.Context, input []byte, files map[string][]byte, args ...string) ([]byte, error) {
	if !m.Available {
		return nil, ErrMagickUnavailable
	}
	base := []string{
		"-limit", "memory", "512MiB",
		"-limit", "map", "1GiB",
		"-limit", "disk", "1GiB",
		"-limit", "area", "128MP",
		"-limit", "time", "150",
	}
	out, err := runSandboxed(ctx, input, files, m.Path, append(base, args...)...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("magick timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("magick failed: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("magick produced no output")
	}
	return out, nil
}

// runSandboxed runs program in the decoder sandbox with a fresh job
// directory holding files, feeding input on stdin. Errors include stderr.
func runSandboxed(ctx context.Context, input []byte, files map[string][]byte, program string, args ...string) ([]byte, error) {
	if _, err := supportDir(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "mayuri-magick-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(name)), data, 0o600); err != nil {
			return nil, err
		}
	}
	cmd, err := sandbox.Command(ctx, dir, program, args...)
	if err != nil {
		return nil, err
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// InputSpec returns the stdin spec with an explicit coder for data.
func InputSpec(data []byte) (string, error) {
	f := Sniff(data)
	switch f {
	case "":
		return "", ErrNotImage
	case "mp4":
		return "", ErrVideoInput
	case "mov", "webm":
		return "", ErrUnsupportedVideo
	}
	return f + ":-", nil
}
