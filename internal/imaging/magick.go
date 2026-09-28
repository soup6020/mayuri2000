package imaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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
	out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
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
	cmd := exec.CommandContext(ctx, m.Path, append(base, args...)...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("magick timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("magick failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("magick produced no output: %s", strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// InputSpec returns the stdin spec with an explicit coder for data.
func InputSpec(data []byte) (string, error) {
	f := Sniff(data)
	if f == "" {
		return "", ErrNotImage
	}
	return f + ":-", nil
}
