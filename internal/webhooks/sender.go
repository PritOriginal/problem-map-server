package webhooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Header names of a delivery.
const (
	HeaderWebhookID = "X-Webhook-Id"
	HeaderEventID   = "X-Event-Id"
	HeaderSignature = "X-Signature"
	HeaderTimestamp = "X-Timestamp"
	HeaderUserAgent = "problem-map-webhooks/1.0"
)

// maxResponseBody bounds how much of an error response is read into the
// delivery log.
const maxResponseBody = 1 << 10

// Request is one delivery attempt.
type Request struct {
	WebhookID int
	URL       string
	Secret    string
	EventID   string
	Body      []byte
}

// Result is the outcome of an attempt: StatusCode is 0 when no response
// was received (network error, timeout, forbidden target); Err carries the
// reason of a failure (a non-2xx status is a failure too).
type Result struct {
	StatusCode int
	Err        error
}

// OK reports whether the receiver acknowledged the delivery (2xx).
func (r Result) OK() bool { return r.Err == nil }

// ErrRedirect is returned when the receiver answered with a redirect:
// redirects are never followed.
var ErrRedirect = errors.New("redirect not followed")

// Sender delivers signed payloads over HTTPS.
type Sender struct {
	client *http.Client
	policy URLPolicy
	now    func() time.Time
}

// SenderOptions configure NewSender.
type SenderOptions struct {
	// Timeout bounds one attempt (connect + request + response).
	Timeout time.Duration
	// Policy validates and pins the target addresses (SSRF guard).
	Policy URLPolicy
	// Transport overrides the HTTP transport (tests); the policy dialer is
	// not applied to it.
	Transport http.RoundTripper
}

// NewSender builds a Sender with a dedicated transport: no redirects, the
// policy dialer, and opts.Timeout as the overall attempt timeout.
func NewSender(opts SenderOptions) *Sender {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	transport := opts.Transport
	if transport == nil {
		dialer := &net.Dialer{Timeout: opts.Timeout}
		transport = &http.Transport{
			Proxy:                 nil, // never through an environment proxy
			DialContext:           opts.Policy.DialContext(dialer),
			TLSHandshakeTimeout:   opts.Timeout,
			ResponseHeaderTimeout: opts.Timeout,
			MaxIdleConns:          16,
			IdleConnTimeout:       time.Minute,
		}
	}
	return &Sender{
		client: &http.Client{
			Transport: transport,
			Timeout:   opts.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return ErrRedirect
			},
		},
		policy: opts.Policy,
		now:    time.Now,
	}
}

// Send POSTs req.Body to req.URL with the signature headers (X-Signature
// covers X-Timestamp and the body, see Sign). The URL is re-validated on every attempt so a webhook whose host started resolving
// to an internal address is no longer called.
func (s *Sender) Send(ctx context.Context, req Request) Result {
	if err := s.policy.Validate(ctx, req.URL); err != nil {
		return Result{Err: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return Result{Err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", HeaderUserAgent)
	httpReq.Header.Set(HeaderWebhookID, strconv.Itoa(req.WebhookID))
	httpReq.Header.Set(HeaderEventID, req.EventID)
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	httpReq.Header.Set(HeaderTimestamp, timestamp)
	httpReq.Header.Set(HeaderSignature, Sign(req.Secret, timestamp, req.Body))

	resp, err := s.client.Do(httpReq)
	if err != nil {
		// A refused redirect still carries the response (body closed).
		res := Result{Err: err}
		if resp != nil {
			res.StatusCode = resp.StatusCode
			_ = resp.Body.Close()
		}
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain a little so the connection can be reused; the rest is discarded.
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{StatusCode: resp.StatusCode, Err: fmt.Errorf("unexpected status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))}
	}
	return Result{StatusCode: resp.StatusCode}
}
