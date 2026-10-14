package imaging

import (
	"strings"
	"testing"
)

func TestMagickPolicy(t *testing.T) {
	m := magickOrSkip(t)

	out, err := runSandboxed(t.Context(), nil, nil, m.Path, "-list", "policy")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := supportDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), dir) || !strings.Contains(string(out), "pattern: @*") {
		t.Fatalf("our policy isn't loaded:\n%s", out)
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`)
	for name, args := range map[string][]string{
		"svg input":     {"svg:-", "png:-"},
		"mvg input":     {"mvg:-", "png:-"},
		"url input":     {"https://example.invalid/x.png", "png:-"},
		"indirect read": {"-size", "100x100", "caption:@secret.txt", "png:-"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := m.RunWith(t.Context(), svg, map[string][]byte{"secret.txt": []byte("hunter2")}, args...)
			if err == nil || !strings.Contains(err.Error(), "not authorized") {
				t.Fatalf("expected a policy refusal, got %v", err)
			}
		})
	}

	// The formats the effects use still work.
	png, err := m.Run(t.Context(), nil, "-size", "8x8", "xc:red", "png:-")
	if err == nil {
		t.Fatalf("xc: should be refused by the policy, got %d bytes", len(png))
	}
	if _, err := m.Run(t.Context(), nil, "-size", "64x32", "-background", "white", "-font", mustArial(t), "caption:hi", "gif:-"); err != nil {
		t.Fatalf("caption to GIF: %v", err)
	}
}

func mustArial(t *testing.T) string {
	t.Helper()
	p, err := arialPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
