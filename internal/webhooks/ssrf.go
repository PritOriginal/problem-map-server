package webhooks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// ErrForbiddenTarget is returned for URLs that must not be called: not
// https, no host, or a host resolving to a loopback/private/link-local
// address (SSRF guard).
var ErrForbiddenTarget = errors.New("forbidden webhook target")

// Resolver looks up the addresses of a host; net.DefaultResolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// URLPolicy validates webhook targets. The zero value requires https and
// rejects private addresses.
type URLPolicy struct {
	// AllowPrivate lifts the address check (local development only).
	AllowPrivate bool
	// Resolver used to check the addresses a host name points to; nil
	// means net.DefaultResolver.
	Resolver Resolver
}

// ParseURL checks the syntax of raw (absolute https URL with a host and
// no credentials) without touching the network.
func ParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrForbiddenTarget, err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: only https urls are accepted", ErrForbiddenTarget)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: host is required", ErrForbiddenTarget)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: credentials in url are not allowed", ErrForbiddenTarget)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("%w: fragment is not allowed", ErrForbiddenTarget)
	}
	return u, nil
}

// Validate checks raw with ParseURL and, unless AllowPrivate, resolves the
// host and rejects it when any address is not publicly routable.
func (p URLPolicy) Validate(ctx context.Context, raw string) error {
	u, err := ParseURL(raw)
	if err != nil {
		return err
	}
	if p.AllowPrivate {
		return nil
	}
	if _, err := p.resolve(ctx, u.Hostname()); err != nil {
		return err
	}
	return nil
}

// resolve returns the public addresses of host, or ErrForbiddenTarget when
// the host is (or resolves to) a non-public address.
func (p URLPolicy) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !IsPublicAddr(addr) {
			return nil, fmt.Errorf("%w: %s is not a public address", ErrForbiddenTarget, addr)
		}
		return []netip.Addr{addr}, nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, fmt.Errorf("%w: %s is not a public host", ErrForbiddenTarget, host)
	}

	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve %s: %w", ErrForbiddenTarget, host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: %s does not resolve", ErrForbiddenTarget, host)
	}
	for _, addr := range addrs {
		if !IsPublicAddr(addr) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrForbiddenTarget, host, addr)
		}
	}
	return addrs, nil
}

// IsPublicAddr reports whether addr is a globally routable unicast address:
// loopback, private (RFC 1918 / ULA), link-local, multicast, unspecified,
// CGNAT (100.64/10), the IPv4-mapped forms of those and the IPv6 transition
// ranges that embed an IPv4 address (NAT64, 6to4, Teredo) are rejected.
func IsPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	switch {
	case !addr.IsValid(),
		addr.IsLoopback(),
		addr.IsPrivate(),
		addr.IsLinkLocalUnicast(),
		addr.IsLinkLocalMulticast(),
		addr.IsInterfaceLocalMulticast(),
		addr.IsMulticast(),
		addr.IsUnspecified():
		return false
	}
	for _, block := range reservedBlocks {
		if block.Contains(addr) {
			return false
		}
	}
	return true
}

var reservedBlocks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, incl. broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64
	netip.MustParsePrefix("2001::/32"),       // Teredo (embeds an IPv4 address)
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("2002::/16"),       // 6to4 (embeds an IPv4 address)
}

// DialContext returns a dialer that resolves the host itself, applies the
// policy to every address (so a DNS answer that changed since validation
// cannot point the request at an internal service) and connects to one of
// the allowed addresses only.
func (p URLPolicy) DialContext(base *net.Dialer) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if p.AllowPrivate {
			return base.DialContext(ctx, network, address)
		}
		addrs, err := p.resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, addr := range addrs {
			conn, err := base.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}
