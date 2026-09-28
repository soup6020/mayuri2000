package safehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestAllowed(t *testing.T) {
	for addr, want := range map[string]bool{
		"1.1.1.1":          true,
		"2606:4700::1111":  true,
		"127.0.0.1":        false,
		"::1":              false,
		"10.0.0.5":         false,
		"172.16.3.4":       false,
		"192.168.1.1":      false,
		"169.254.169.254":  false, // cloud metadata
		"100.100.1.1":      false, // CGNAT / Tailscale
		"0.0.0.0":          false,
		"fd00::1":          false,
		"fe80::1":          false,
		"::ffff:127.0.0.1": false, // IPv4-mapped loopback
		"64:ff9b::a00:1":   false,
		"224.0.0.1":        false,
	} {
		if got := Allowed(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Allowed(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestClientRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secret"))
	}))
	defer srv.Close()

	get := func(u string) error {
		req, err := NewRequest(context.Background(), u)
		if err != nil {
			return err
		}
		resp, err := NewClient(5 * time.Second).Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}
	if err := get(srv.URL); !errors.Is(err, ErrForbiddenAddress) {
		t.Fatalf("expected ErrForbiddenAddress, got %v", err)
	}
	// localhost by name resolves to loopback and must be refused too.
	if err := get(strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)); !errors.Is(err, ErrForbiddenAddress) {
		t.Fatalf("localhost: expected ErrForbiddenAddress, got %v", err)
	}
}

func TestParseURL(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "gopher://x", "ftp://example.com/a", "http://user:pw@example.com/", "http:///nohost", "::::"} {
		if _, err := ParseURL(bad); err == nil {
			t.Errorf("ParseURL(%q) accepted", bad)
		}
	}
	if _, err := ParseURL("https://example.com/feed.xml"); err != nil {
		t.Errorf("valid URL rejected: %v", err)
	}
}
