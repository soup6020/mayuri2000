package config

import (
	"strings"
	"testing"

	"mayuri2000/internal/sandbox"
)

func TestDecoderSandbox(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "x")
	for value, want := range map[string]sandbox.Mode{"": sandbox.Required, "required": sandbox.Required, "best-effort": sandbox.BestEffort} {
		t.Setenv("DECODER_SANDBOX", value)
		cfg, err := Load()
		if err != nil || cfg.DecoderSandbox != want {
			t.Errorf("DECODER_SANDBOX=%q: %v, %v", value, cfg, err)
		}
	}
	t.Setenv("DECODER_SANDBOX", "off")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DECODER_SANDBOX must be required or best-effort") {
		t.Fatalf("invalid mode: %v", err)
	}
}
