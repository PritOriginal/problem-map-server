package webhooks_test

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/webhooks"
	"github.com/stretchr/testify/suite"
)

// fakeResolver answers from a fixed table; unknown hosts fail.
type fakeResolver map[string][]string

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	ips, ok := r[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, netip.MustParseAddr(ip))
	}
	return addrs, nil
}

type SSRFSuite struct {
	suite.Suite
}

func TestSSRF(t *testing.T) {
	suite.Run(t, new(SSRFSuite))
}

func (suite *SSRFSuite) TestIsPublicAddr() {
	tests := []struct {
		addr string
		want bool
	}{
		{"93.184.216.34", true},
		{"2606:2800:220:1:248:1893:25c8:1946", true},
		{"127.0.0.1", false},
		{"127.10.0.1", false},
		{"::1", false},
		{"10.0.0.5", false},
		{"172.16.3.4", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false}, // cloud metadata
		{"fe80::1", false},
		{"fc00::1", false},
		{"100.64.0.1", false},
		{"0.0.0.0", false},
		{"::", false},
		{"224.0.0.1", false},
		{"255.255.255.255", false},
		{"::ffff:10.0.0.1", false}, // IPv4-mapped private
		{"::ffff:93.184.216.34", true},
		{"64:ff9b::7f00:1", false}, // NAT64 of 127.0.0.1
		{"2001:db8::1", false},
		{"2002:7f00:1::1", false},     // 6to4 of 127.0.0.1
		{"2001:0:0:0::7f00:1", false}, // Teredo
		{"::ffff:10.0.0.1", false},    // IPv4-mapped private
		{"fd00::1", false},            // ULA
		{"fe80::1%eth0", false},       // link-local with zone
	}
	for _, tt := range tests {
		suite.Run(tt.addr, func() {
			suite.Equal(tt.want, webhooks.IsPublicAddr(netip.MustParseAddr(tt.addr)))
		})
	}
}

func (suite *SSRFSuite) TestParseURL() {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "Https", raw: "https://example.org/hook"},
		{name: "HttpsWithPortAndQuery", raw: "https://example.org:8443/hook?token=1"},
		{name: "Http", raw: "http://example.org/hook", wantErr: true},
		{name: "NoScheme", raw: "example.org/hook", wantErr: true},
		{name: "NoHost", raw: "https:///hook", wantErr: true},
		{name: "Credentials", raw: "https://user:pass@example.org/hook", wantErr: true},
		{name: "Fragment", raw: "https://example.org/hook#x", wantErr: true},
		{name: "Garbage", raw: "https://exa mple.org", wantErr: true},
		{name: "File", raw: "file:///etc/passwd", wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			_, err := webhooks.ParseURL(tt.raw)
			if tt.wantErr {
				suite.ErrorIs(err, webhooks.ErrForbiddenTarget)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *SSRFSuite) TestURLPolicyValidate() {
	resolver := fakeResolver{
		"public.example":   {"93.184.216.34"},
		"dual.example":     {"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"},
		"internal.example": {"10.0.0.5"},
		"mixed.example":    {"93.184.216.34", "10.0.0.5"}, // DNS rebinding style answer
		"metadata.example": {"169.254.169.254"},
		"nowhere.example":  {},
	}
	policy := webhooks.URLPolicy{Resolver: resolver}
	permissive := webhooks.URLPolicy{Resolver: resolver, AllowPrivate: true}

	tests := []struct {
		name    string
		policy  webhooks.URLPolicy
		raw     string
		wantErr bool
	}{
		{name: "PublicHost", policy: policy, raw: "https://public.example/hook"},
		{name: "DualStackPublic", policy: policy, raw: "https://dual.example/hook"},
		{name: "PublicLiteralIP", policy: policy, raw: "https://93.184.216.34/hook"},
		{name: "PublicLiteralIPv6", policy: policy, raw: "https://[2606:2800:220:1:248:1893:25c8:1946]/hook"},
		{name: "InternalHost", policy: policy, raw: "https://internal.example/hook", wantErr: true},
		{name: "MixedAnswer", policy: policy, raw: "https://mixed.example/hook", wantErr: true},
		{name: "Metadata", policy: policy, raw: "https://metadata.example/hook", wantErr: true},
		{name: "Localhost", policy: policy, raw: "https://localhost/hook", wantErr: true},
		{name: "LocalhostSubdomain", policy: policy, raw: "https://api.localhost/hook", wantErr: true},
		{name: "LoopbackLiteral", policy: policy, raw: "https://127.0.0.1:8443/hook", wantErr: true},
		{name: "LoopbackLiteralIPv6", policy: policy, raw: "https://[::1]/hook", wantErr: true},
		{name: "PrivateLiteral", policy: policy, raw: "https://192.168.0.10/hook", wantErr: true},
		{name: "Unresolvable", policy: policy, raw: "https://unknown.example/hook", wantErr: true},
		{name: "EmptyAnswer", policy: policy, raw: "https://nowhere.example/hook", wantErr: true},
		{name: "HttpStillRejectedWhenPermissive", policy: permissive, raw: "http://127.0.0.1/hook", wantErr: true},
		{name: "PrivateAllowedWhenPermissive", policy: permissive, raw: "https://127.0.0.1/hook"},
		{name: "InternalHostAllowedWhenPermissive", policy: permissive, raw: "https://internal.example/hook"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := tt.policy.Validate(context.Background(), tt.raw)
			if tt.wantErr {
				suite.ErrorIs(err, webhooks.ErrForbiddenTarget)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *SSRFSuite) TestDialContextRejectsPrivate() {
	policy := webhooks.URLPolicy{Resolver: fakeResolver{"internal.example": {"10.0.0.5"}}}
	dial := policy.DialContext(nil)

	_, err := dial(context.Background(), "tcp", "internal.example:443")
	suite.ErrorIs(err, webhooks.ErrForbiddenTarget)

	_, err = dial(context.Background(), "tcp", "127.0.0.1:443")
	suite.ErrorIs(err, webhooks.ErrForbiddenTarget)
}
