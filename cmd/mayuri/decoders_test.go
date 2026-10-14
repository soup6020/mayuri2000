package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"testing"

	"mayuri2000/internal/config"
	"mayuri2000/internal/sandbox"
)

func TestStartDecoders(t *testing.T) {
	t.Cleanup(func() { sandbox.SetMode(sandbox.Required) })
	magickPath, err := exec.LookPath("magick")
	if err != nil {
		t.Skip("magick not on PATH")
	}
	cfg := &config.Config{MagickPath: magickPath, FFmpegPath: "ffmpeg", DecoderSandbox: sandbox.Required}
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	// Required mode, sandbox unavailable: fail closed.
	failing := func(context.Context) (sandbox.Report, error) {
		return sandbox.Report{Problems: []string{"Landlock is not available in this kernel"}}, errors.New("decoder sandbox unavailable")
	}
	m, ff := startDecoders(t.Context(), cfg, log, failing)
	if m.Available || ff.Available {
		t.Fatal("decoders enabled without a sandbox in required mode")
	}
	if !strings.Contains(logs.String(), "decoder sandbox unavailable") {
		t.Fatalf("no warning logged: %s", logs.String())
	}

	// Working sandbox: decoders run (sandboxed) and are enabled.
	logs.Reset()
	m, _ = startDecoders(t.Context(), cfg, log, sandbox.Probe)
	if sandbox.LandlockABI() >= 1 && (!m.Available || !strings.Contains(logs.String(), "decoder sandbox active")) {
		t.Fatalf("magick available=%v; logs: %s", m.Available, logs.String())
	}
}
