package apns_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/internal/push/apns"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/suite"
)

const (
	keyID    = "ABC123DEFG"
	teamID   = "TEAM123456"
	bundleID = "ru.problem-map.app"
)

// generateKey makes a P-256 key and its PKCS#8 PEM, as in a .p8 file.
func generateKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func apnsError(reason string) []byte {
	raw, _ := json.Marshal(map[string]string{"reason": reason})
	return raw
}

// response is one scripted answer of the fake APNs server.
type response struct {
	status     int
	body       []byte
	retryAfter string
}

// fakeAPNs answers requests from a script, one response per request (the
// last one repeats), and records what it received.
type fakeAPNs struct {
	mu       sync.Mutex
	script   []response
	requests []*http.Request
	bodies   []map[string]any
	inFlight atomic.Int32
	maxSeen  atomic.Int32
}

func (f *fakeAPNs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		seen := f.maxSeen.Load()
		if n <= seen || f.maxSeen.CompareAndSwap(seen, n) {
			break
		}
	}
	// Let concurrent requests overlap so the concurrency limit is observable.
	time.Sleep(20 * time.Millisecond)

	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.bodies = append(f.bodies, body)
	i := min(len(f.requests)-1, len(f.script)-1)
	resp := f.script[i]
	f.mu.Unlock()

	if resp.retryAfter != "" {
		w.Header().Set("Retry-After", resp.retryAfter)
	}
	w.WriteHeader(resp.status)
	_, _ = w.Write(resp.body)
}

func (f *fakeAPNs) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// bearer returns the provider token of the i-th recorded request.
func (f *fakeAPNs) bearer(i int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.TrimPrefix(f.requests[i].Header.Get("Authorization"), "bearer ")
}

// newServer starts a TLS HTTP/2 server, as APNs requires.
func newServer(h http.Handler) *httptest.Server {
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	return srv
}

type APNsSuite struct {
	suite.Suite
	key *ecdsa.PrivateKey
	pem string
}

func TestAPNs(t *testing.T) {
	suite.Run(t, new(APNsSuite))
}

func (suite *APNsSuite) SetupSuite() {
	suite.key, suite.pem = generateKey(suite.T())
}

func (suite *APNsSuite) newSender(server *httptest.Server, cfg config.APNsConfig, opts ...apns.Option) *apns.Sender {
	if cfg.KeyP8 == "" && cfg.KeyFile == "" {
		cfg.KeyP8 = suite.pem
	}
	if cfg.KeyID == "" {
		cfg.KeyID = keyID
	}
	if cfg.TeamID == "" {
		cfg.TeamID = teamID
	}
	if cfg.BundleID == "" {
		cfg.BundleID = bundleID
	}
	if cfg.Environment == "" {
		cfg.Environment = config.APNsProduction
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = time.Second
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 8
	}
	// server.Client() trusts the test certificate and forces HTTP/2.
	opts = append([]apns.Option{
		apns.WithBaseURL(server.URL),
		apns.WithHTTPClient(server.Client()),
		apns.WithBackoff(time.Millisecond),
	}, opts...)
	sender, err := apns.New(slogdiscard.NewDiscardLogger(), cfg, opts...)
	suite.Require().NoError(err)
	return sender
}

func (suite *APNsSuite) TestNew() {
	_, ecPEM := generateKey(suite.T())
	rsaKey := func() string {
		// An RSA key in PKCS#8: right container, wrong algorithm.
		der, err := x509.MarshalPKCS8PrivateKey(mustRSA(suite.T()))
		suite.Require().NoError(err)
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	}

	tests := []struct {
		name    string
		cfg     config.APNsConfig
		wantErr string
	}{
		{name: "P8", cfg: config.APNsConfig{KeyP8: ecPEM}},
		{name: "NotPEM", cfg: config.APNsConfig{KeyP8: "garbage"}, wantErr: "not PEM"},
		{name: "RSAKey", cfg: config.APNsConfig{KeyP8: rsaKey()}, wantErr: "want ECDSA"},
		{name: "MissingFile", cfg: config.APNsConfig{KeyFile: "/nonexistent/AuthKey.p8"}, wantErr: "read key file"},
		{name: "NoKey", cfg: config.APNsConfig{}, wantErr: "not configured"},
		{name: "NoIDs", cfg: config.APNsConfig{KeyP8: ecPEM, KeyID: "", TeamID: "", BundleID: ""}, wantErr: "APNS_KEY_ID"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			cfg := tt.cfg
			if tt.name != "NoIDs" {
				cfg.KeyID, cfg.TeamID, cfg.BundleID = keyID, teamID, bundleID
			}
			cfg.Environment, cfg.Timeout, cfg.Concurrency = config.APNsProduction, time.Second, 1
			_, err := apns.New(slogdiscard.NewDiscardLogger(), cfg)
			if tt.wantErr == "" {
				suite.NoError(err)
				return
			}
			suite.ErrorContains(err, tt.wantErr)
		})
	}
}

func (suite *APNsSuite) TestSend() {
	device := models.UserDevice{UserID: 7, Platform: models.PlatformIOS, Token: "deadbeef"}
	ok := []byte(nil)

	tests := []struct {
		name         string
		script       []response
		maxRetries   int
		wantErr      error
		wantAnyErr   bool
		wantRequests int
		wantNewToken bool
	}{
		{
			name:         "Ok",
			script:       []response{{status: http.StatusOK, body: ok}},
			maxRetries:   3,
			wantRequests: 1,
		},
		{
			name: "ServerErrorThenOk",
			script: []response{
				{status: http.StatusInternalServerError, body: apnsError("InternalServerError")},
				{status: http.StatusServiceUnavailable, body: apnsError("ServiceUnavailable")},
				{status: http.StatusOK, body: ok},
			},
			maxRetries:   3,
			wantRequests: 3,
		},
		{
			name:         "ServerErrorExhaustsRetries",
			script:       []response{{status: http.StatusInternalServerError, body: apnsError("InternalServerError")}},
			maxRetries:   3,
			wantAnyErr:   true,
			wantRequests: 4,
		},
		{
			name: "TooManyRequestsRetried",
			script: []response{
				{status: http.StatusTooManyRequests, body: apnsError("TooManyRequests"), retryAfter: "0"},
				{status: http.StatusOK, body: ok},
			},
			maxRetries:   3,
			wantRequests: 2,
		},
		{
			name:         "NoRetriesConfigured",
			script:       []response{{status: http.StatusBadGateway, body: nil}},
			maxRetries:   0,
			wantAnyErr:   true,
			wantRequests: 1,
		},
		{
			name:         "Unregistered",
			script:       []response{{status: http.StatusGone, body: apnsError(apns.ReasonUnregistered)}},
			maxRetries:   3,
			wantErr:      push.ErrInvalidToken,
			wantRequests: 1,
		},
		{
			name:         "BadDeviceToken",
			script:       []response{{status: http.StatusBadRequest, body: apnsError(apns.ReasonBadDeviceToken)}},
			maxRetries:   3,
			wantErr:      push.ErrInvalidToken,
			wantRequests: 1,
		},
		{
			name:         "DeviceTokenNotForTopic",
			script:       []response{{status: http.StatusBadRequest, body: apnsError(apns.ReasonDeviceTokenNotForTopic)}},
			maxRetries:   3,
			wantErr:      push.ErrInvalidToken,
			wantRequests: 1,
		},
		{
			name: "ExpiredProviderTokenResigned",
			script: []response{
				{status: http.StatusForbidden, body: apnsError(apns.ReasonExpiredProviderToken)},
				{status: http.StatusOK, body: ok},
			},
			maxRetries:   0,
			wantRequests: 2,
			wantNewToken: true,
		},
		{
			name:         "ExpiredProviderTokenTwiceFails",
			script:       []response{{status: http.StatusForbidden, body: apnsError(apns.ReasonExpiredProviderToken)}},
			maxRetries:   3,
			wantAnyErr:   true,
			wantRequests: 2,
		},
		{
			name:         "OtherClientErrorNotRetried",
			script:       []response{{status: http.StatusForbidden, body: apnsError("InvalidProviderToken")}},
			maxRetries:   3,
			wantAnyErr:   true,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			fake := &fakeAPNs{script: tt.script}
			server := newServer(fake)
			defer server.Close()

			// A ticking clock so a re-signed token differs in iat.
			var tick atomic.Int64
			clock := func() time.Time { return time.Now().Add(time.Duration(tick.Add(1)) * time.Second) }
			sender := suite.newSender(server, config.APNsConfig{MaxRetries: tt.maxRetries}, apns.WithClock(clock))
			err := sender.Send(context.Background(), device, models.Notification{ID: 1, Type: models.NotificationTaskAssigned, Title: "t"})

			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			case tt.wantAnyErr:
				suite.Error(err)
				suite.NotErrorIs(err, push.ErrInvalidToken)
			default:
				suite.NoError(err)
			}
			suite.Equal(tt.wantRequests, fake.count())
			if tt.wantNewToken {
				suite.NotEqual(fake.bearer(0), fake.bearer(1), "a fresh token after ExpiredProviderToken")
			}
		})
	}
}

func (suite *APNsSuite) TestRequestAndToken() {
	fake := &fakeAPNs{script: []response{{status: http.StatusOK}}}
	server := newServer(fake)
	defer server.Close()

	now := time.Now().Truncate(time.Second)
	sender := suite.newSender(server, config.APNsConfig{}, apns.WithClock(func() time.Time { return now }))

	n := models.Notification{
		ID: 42, UserID: 7, Type: models.NotificationTaskAssigned,
		MarkID: null.IntFrom(3), TaskID: null.IntFrom(9), Title: "Title", Body: "Body",
	}
	suite.Require().NoError(sender.Send(context.Background(), models.UserDevice{Platform: models.PlatformIOS, Token: "dev1ce"}, n))
	suite.Require().Len(fake.requests, 1)

	req := fake.requests[0]
	suite.Equal(2, req.ProtoMajor, "APNs requires HTTP/2")
	suite.Equal(http.MethodPost, req.Method)
	suite.Equal("/3/device/dev1ce", req.URL.Path)
	suite.Equal(bundleID, req.Header.Get("apns-topic"))
	suite.Equal("alert", req.Header.Get("apns-push-type"))
	suite.Equal("10", req.Header.Get("apns-priority"))
	suite.Equal("42", req.Header.Get("apns-collapse-id"))
	suite.Equal(now.Add(apns.Expiration).Unix(), mustInt(suite.T(), req.Header.Get("apns-expiration")))
	suite.Contains(req.Header.Get("Content-Type"), "application/json")

	suite.Equal(map[string]any{
		"aps":             map[string]any{"alert": map[string]any{"title": "Title", "body": "Body"}, "sound": "default"},
		"type":            string(models.NotificationTaskAssigned),
		"notification_id": "42",
		"mark_id":         "3",
		"task_id":         "9",
	}, fake.bodies[0])

	// The provider token: ES256 JWT by the key, with kid/iss/iat.
	token := fake.bearer(0)
	parts := strings.Split(token, ".")
	suite.Require().Len(parts, 3, "jwt: %s", token)

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	suite.Require().NoError(json.Unmarshal(b64(suite.T(), parts[0]), &header))
	suite.Equal("ES256", header.Alg)
	suite.Equal(keyID, header.Kid)

	var claims struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
	}
	suite.Require().NoError(json.Unmarshal(b64(suite.T(), parts[1]), &claims))
	suite.Equal(teamID, claims.Iss)
	suite.Equal(now.Unix(), claims.Iat)

	signature := b64(suite.T(), parts[2])
	suite.Require().Len(signature, 64, "raw r||s signature")
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	suite.True(ecdsa.Verify(&suite.key.PublicKey, digest[:], r, s), "signature verifies with the key")
}

func (suite *APNsSuite) TestTokenCache() {
	fake := &fakeAPNs{script: []response{{status: http.StatusOK}}}
	server := newServer(fake)
	defer server.Close()

	now := time.Now()
	var offset atomic.Int64 // seconds added to now
	clock := func() time.Time { return now.Add(time.Duration(offset.Load()) * time.Second) }
	sender := suite.newSender(server, config.APNsConfig{}, apns.WithClock(clock))

	device := models.UserDevice{Platform: models.PlatformIOS, Token: "tok"}
	n := models.Notification{ID: 1, Type: models.NotificationTaskAssigned, Title: "t"}

	suite.Require().NoError(sender.Send(context.Background(), device, n))
	offset.Store(int64((apns.TokenTTL - time.Minute).Seconds()))
	suite.Require().NoError(sender.Send(context.Background(), device, n))
	suite.Equal(fake.bearer(0), fake.bearer(1), "the token is reused within its ttl")

	offset.Store(int64((apns.TokenTTL + time.Minute).Seconds()))
	suite.Require().NoError(sender.Send(context.Background(), device, n))
	suite.NotEqual(fake.bearer(1), fake.bearer(2), "the token is re-signed after its ttl")
}

func (suite *APNsSuite) TestConcurrencyLimit() {
	fake := &fakeAPNs{script: []response{{status: http.StatusOK}}}
	server := newServer(fake)
	defer server.Close()

	const limit = 2
	sender := suite.newSender(server, config.APNsConfig{Concurrency: limit})

	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := sender.Send(context.Background(), models.UserDevice{Platform: models.PlatformIOS, Token: "tok"},
				models.Notification{ID: i, Type: models.NotificationTaskAssigned, Title: "t"})
			suite.NoError(err)
		}()
	}
	wg.Wait()

	suite.Equal(6, fake.count())
	suite.LessOrEqual(fake.maxSeen.Load(), int32(limit))
}

func (suite *APNsSuite) TestContextCancelled() {
	fake := &fakeAPNs{script: []response{{status: http.StatusInternalServerError, body: apnsError("InternalServerError")}}}
	server := newServer(fake)
	defer server.Close()

	sender := suite.newSender(server, config.APNsConfig{MaxRetries: 3}, apns.WithBackoff(time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := sender.Send(ctx, models.UserDevice{Platform: models.PlatformIOS, Token: "tok"},
		models.Notification{ID: 1, Type: models.NotificationTaskAssigned, Title: "t"})
	suite.ErrorIs(err, context.DeadlineExceeded)
	suite.Equal(1, fake.count())
}

func b64(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("base64url %q: %v", s, err)
	}
	return raw
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("int %q: %v", s, err)
	}
	return v
}

func mustRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
