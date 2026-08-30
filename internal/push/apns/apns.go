// Package apns sends pushes through Apple Push Notification service over
// HTTP/2 with token-based authentication: every request carries a JWT
// signed (ES256) by the .p8 key of the developer account; the token is
// cached and re-signed before Apple's one-hour limit.
package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
)

const (
	// ProductionURL and SandboxURL are the APNs hosts of the two environments.
	ProductionURL = "https://api.push.apple.com"
	SandboxURL    = "https://api.sandbox.push.apple.com"

	// TokenTTL is how long a signed provider token is reused. Apple rejects
	// tokens older than an hour (ExpiredProviderToken) and asks not to
	// refresh more often than every 20 minutes.
	TokenTTL = 50 * time.Minute
	// Expiration is how long APNs keeps an undeliverable push for an
	// offline device (apns-expiration).
	Expiration = 24 * time.Hour

	// maxErrorBody bounds how much of an error response is read.
	maxErrorBody = 64 << 10
)

// Reasons of an APNs error response (https://developer.apple.com/documentation/usernotifications/handling-notification-responses-from-apns).
const (
	ReasonBadDeviceToken         = "BadDeviceToken"
	ReasonUnregistered           = "Unregistered"
	ReasonDeviceTokenNotForTopic = "DeviceTokenNotForTopic"
	ReasonExpiredProviderToken   = "ExpiredProviderToken"
)

// errExpiredToken marks a 403 ExpiredProviderToken: the cached JWT must be
// re-signed and the request repeated once.
var errExpiredToken = errors.New("apns: provider token expired")

// Sender delivers pushes to ios devices through APNs.
type Sender struct {
	log        *slog.Logger
	client     *http.Client
	url        string
	topic      string
	sem        chan struct{}
	timeout    time.Duration
	maxRetries int
	backoff    time.Duration
	now        func() time.Time

	key    *ecdsa.PrivateKey
	keyID  string
	teamID string

	mu          sync.Mutex
	token       string
	tokenIssued time.Time
}

// Option customises the Sender (mostly for tests).
type Option func(*Sender)

// WithHTTPClient replaces the HTTP client (its Timeout is not used: every
// request is bounded by the configured timeout via context). The client
// must speak HTTP/2: APNs refuses HTTP/1.1.
func WithHTTPClient(c *http.Client) Option { return func(s *Sender) { s.client = c } }

// WithBaseURL points the sender at another host (an httptest.Server)
// instead of the environment's one.
func WithBaseURL(base string) Option { return func(s *Sender) { s.url = base } }

// WithBackoff sets the base delay between retries (doubled on every retry).
func WithBackoff(d time.Duration) Option { return func(s *Sender) { s.backoff = d } }

// WithClock replaces the clock used for the token's iat and its cache.
func WithClock(now func() time.Time) Option { return func(s *Sender) { s.now = now } }

// New builds the sender from cfg: the signing key is read from KeyFile or
// KeyP8 and must be a P-256 ECDSA key (the .p8 Apple issues).
func New(log *slog.Logger, cfg config.APNsConfig, opts ...Option) (*Sender, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, errors.New("apns: signing key is not configured (push.apns.key-file / key-p8)")
	}

	raw, err := readKey(cfg)
	if err != nil {
		return nil, err
	}
	key, err := ParseKey(raw)
	if err != nil {
		return nil, err
	}

	s := &Sender{
		log:        log.With(slog.String("component", "push.apns")),
		client:     &http.Client{Transport: newTransport()},
		url:        ProductionURL,
		topic:      cfg.BundleID,
		sem:        make(chan struct{}, cfg.Concurrency),
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		backoff:    push.DefaultBackoff,
		now:        time.Now,
		key:        key,
		keyID:      cfg.KeyID,
		teamID:     cfg.TeamID,
	}
	if cfg.Environment == config.APNsSandbox {
		s.url = SandboxURL
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// newTransport is the HTTP/2 transport APNs requires: with TLS the
// protocol is negotiated through ALPN, so a direct connection to Apple is
// always HTTP/2. HTTP(S)_PROXY is honoured like http.DefaultTransport does;
// a CONNECT tunnel keeps TLS (and thus ALPN) end-to-end, but a proxy that
// terminates TLS itself would downgrade to HTTP/1.1 and APNs would refuse
// the request. The idle connection is kept so the TLS handshake is not
// repeated for every push.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        16,
		IdleConnTimeout:     10 * time.Minute,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

func readKey(cfg config.APNsConfig) ([]byte, error) {
	if cfg.KeyP8 != "" {
		return []byte(cfg.KeyP8), nil
	}
	data, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("apns: read key file: %w", err)
	}
	return data, nil
}

// ParseKey decodes a PEM-encoded ECDSA P-256 private key: PKCS#8 (the
// .p8 Apple issues) or SEC 1 ("EC PRIVATE KEY").
func ParseKey(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("apns: signing key is not PEM")
	}

	var parsed any
	var err error
	switch block.Type {
	case "EC PRIVATE KEY":
		parsed, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, fmt.Errorf("apns: parse signing key: %w", err)
	}

	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apns: signing key is %T, want ECDSA", parsed)
	}
	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("apns: signing key curve is %s, want P-256", key.Params().Name)
	}
	return key, nil
}

// providerToken returns the cached JWT, signing a new one when the cached
// one is older than TokenTTL.
func (s *Sender) providerToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.token != "" && now.Sub(s.tokenIssued) < TokenTTL {
		return s.token, nil
	}
	token, err := s.signToken(now)
	if err != nil {
		return "", err
	}
	s.token, s.tokenIssued = token, now
	return token, nil
}

// invalidateToken drops the cached JWT if it is still the given one (a
// concurrent request may already have signed a fresh token).
func (s *Sender) invalidateToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == token {
		s.token, s.tokenIssued = "", time.Time{}
	}
}

// signToken builds the provider token: a JWT with header {alg: ES256,
// kid: KeyID} and claims {iss: TeamID, iat: now}, signed by the .p8 key.
// The JWS signature of ES256 is the raw r||s (32 bytes each), not the
// ASN.1 DER that crypto/ecdsa produces by default.
func (s *Sender) signToken(now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": s.keyID})
	if err != nil {
		return "", fmt.Errorf("apns: encode token header: %w", err)
	}
	claims, err := json.Marshal(map[string]any{"iss": s.teamID, "iat": now.Unix()})
	if err != nil {
		return "", fmt.Errorf("apns: encode token claims: %w", err)
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	r, sig, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("apns: sign token: %w", err)
	}

	const size = 32 // P-256 coordinate size in bytes
	signature := make([]byte, 2*size)
	r.FillBytes(signature[:size])
	sig.FillBytes(signature[size:])

	return signingInput + "." + enc.EncodeToString(signature), nil
}

// payload is the APNs notification body: the aps dictionary plus the
// custom keys of push.Data at the top level.
func payload(n models.Notification) ([]byte, error) {
	body := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": n.Title, "body": n.Body},
			"sound": "default",
		},
	}
	for k, v := range push.Data(n) {
		body[k] = v
	}
	return json.Marshal(body)
}

// errorResponse is the body of a failed APNs request.
type errorResponse struct {
	Reason string `json:"reason"`
}

// Send posts n to the device token, retrying 5xx/429 and transport errors
// up to MaxRetries times with exponential backoff. A token APNs reports as
// bad, unregistered or foreign to the topic yields push.ErrInvalidToken; an
// expired provider token is re-signed and the request repeated once; other
// 4xx are not retried.
func (s *Sender) Send(ctx context.Context, device models.UserDevice, n models.Notification) error {
	body, err := payload(n)
	if err != nil {
		return fmt.Errorf("apns: encode payload: %w", err)
	}

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return fmt.Errorf("apns: %w", ctx.Err())
	}

	refreshed := false
	err = push.Retry(ctx, s.log, s.maxRetries, s.backoff, func(ctx context.Context) (time.Duration, bool, error) {
		for {
			retryAfter, retry, err := s.attempt(ctx, device.Token, n.ID, body)
			if errors.Is(err, errExpiredToken) && !refreshed {
				refreshed = true
				s.log.Warn("apns rejected the provider token as expired, signing a new one")
				continue
			}
			return retryAfter, retry, err
		}
	})
	if err != nil && errors.Is(err, ctx.Err()) {
		return fmt.Errorf("apns: %w", ctx.Err())
	}
	return err
}

// attempt performs one request; retry reports whether the error is
// transient and retryAfter carries the server's Retry-After, if any.
func (s *Sender) attempt(ctx context.Context, deviceToken string, notificationID int, body []byte) (retryAfter time.Duration, retry bool, err error) {
	token, err := s.providerToken()
	if err != nil {
		return 0, false, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, s.url+"/3/device/"+deviceToken, bytes.NewReader(body))
	if err != nil {
		return 0, false, fmt.Errorf("apns: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("apns-topic", s.topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-expiration", strconv.FormatInt(s.now().Add(Expiration).Unix(), 10))
	req.Header.Set("apns-collapse-id", strconv.Itoa(notificationID))

	resp, err := s.client.Do(req)
	if err != nil {
		// The parent context is done: nothing to retry (Send adds the prefix).
		if ctx.Err() != nil {
			return 0, false, ctx.Err()
		}
		return 0, true, fmt.Errorf("apns: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return 0, false, nil
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var apiErr errorResponse
	_ = json.Unmarshal(raw, &apiErr)

	err = fmt.Errorf("apns: status %d %s", resp.StatusCode, apiErr.Reason)
	switch {
	case apiErr.Reason == ReasonBadDeviceToken, apiErr.Reason == ReasonUnregistered, apiErr.Reason == ReasonDeviceTokenNotForTopic:
		return 0, false, fmt.Errorf("%w: %w", push.ErrInvalidToken, err)
	case resp.StatusCode == http.StatusForbidden && apiErr.Reason == ReasonExpiredProviderToken:
		s.invalidateToken(token)
		return 0, false, fmt.Errorf("%w: %w", errExpiredToken, err)
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return push.ParseRetryAfter(resp.Header.Get("Retry-After")), true, err
	default:
		return 0, false, err
	}
}
