// Package safehttp provides an HTTP client for fetching user-supplied URLs.
// It refuses to connect to loopback, private, link-local and other
// non-public addresses, so users can't make the bot probe its host or local
// network (SSRF). The check runs on the resolved IP at dial time, which also
// covers redirects and DNS rebinding.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrForbiddenAddress is returned when a URL resolves to a non-public address.
var ErrForbiddenAddress = errors.New("that address is not allowed")

var extraBlocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this network"
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT / Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64 can reach IPv4 internals
}

// Allowed reports whether ip is a public unicast address.
func Allowed(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, p := range extraBlocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrForbiddenAddress, address)
	}
	if !Allowed(ap.Addr()) {
		return fmt.Errorf("%w: %s", ErrForbiddenAddress, ap.Addr())
	}
	return nil
}

// NewClient returns an HTTP client that only connects to public addresses.
func NewClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: control}
	transport := &http.Transport{
		Proxy:                 nil, // a proxy would bypass the address check
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return CheckURL(req.URL)
		},
	}
}

// CheckURL rejects non-HTTP(S) URLs and URLs with credentials.
func CheckURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported")
	}
	if u.User != nil {
		return fmt.Errorf("URLs with credentials are not supported")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL has no host")
	}
	return nil
}

// ParseURL parses and validates a user-supplied URL.
func ParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	return u, CheckURL(u)
}

// NewRequest builds a GET request for a validated user-supplied URL.
func NewRequest(ctx context.Context, raw string) (*http.Request, error) {
	u, err := ParseURL(raw)
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
}
