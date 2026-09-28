package core

import (
	"bytes"
	"testing"
)

func TestDecodeBase64(t *testing.T) {
	for in, want := range map[string]string{
		"aGVsbG8gd29ybGQ=":      "hello world", // standard, padded
		"aGVsbG8gd29ybGQ":       "hello world", // unpadded
		"aGVs\nbG8g d29y\tbGQ=": "hello world", // wrapped
		"-_8":                   "\xfb\xff",    // URL-safe
		"+/8=":                  "\xfb\xff",    // standard equivalent
	} {
		got, err := DecodeBase64(in)
		if err != nil || !bytes.Equal(got, []byte(want)) {
			t.Errorf("DecodeBase64(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"not base64!", "a", "aGVsbG8*"} {
		if _, err := DecodeBase64(bad); err == nil {
			t.Errorf("DecodeBase64(%q) should fail", bad)
		}
	}
}

func TestIsPrintableText(t *testing.T) {
	for in, want := range map[string]bool{
		"hello\nworld\t!": true,
		"héllo 🎉":         true,
		"\x00\x01binary":  false,
		"\xff\xfe":        false,
	} {
		if got := IsPrintableText([]byte(in)); got != want {
			t.Errorf("IsPrintableText(%q) = %v", in, got)
		}
	}
}
