// Package fcm sends pushes through Firebase Cloud Messaging (HTTP v1 API)
// authenticated by a Google service account (OAuth2 JWT bearer flow).
package fcm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// Scope is the OAuth2 scope required by the FCM v1 API.
	Scope = "https://www.googleapis.com/auth/firebase.messaging"
	// DefaultBaseURL is the FCM API host.
	DefaultBaseURL = "https://fcm.googleapis.com"

	defaultBackoff = 500 * time.Millisecond
	maxBackoff     = 10 * time.Second
	// maxErrorBody bounds how much of an error response is read.
	maxErrorBody = 64 << 10
)

// Sender delivers pushes to android and web devices through FCM.
type Sender struct {
	log        *slog.Logger
	client     *http.Client
	tokens     oauth2.TokenSource
	url        string
	sem        chan struct{}
	timeout    time.Duration
	maxRetries int
	backoff    time.Duration
}

// Option customises the Sender (mostly for tests).
type Option func(*Sender)

// WithHTTPClient replaces the HTTP client (its Timeout is not used: every
// request is bounded by the configured timeout via context).
func WithHTTPClient(c *http.Client) Option { return func(s *Sender) { s.client = c } }

// WithBaseURL points the sender at another host (an httptest.Server).
func WithBaseURL(base string) Option { return func(s *Sender) { s.url = base } }

// WithTokenSource replaces the service-account token source; with it the
// credentials in the config are not read, so ProjectID must be set.
func WithTokenSource(ts oauth2.TokenSource) Option { return func(s *Sender) { s.tokens = ts } }

// WithBackoff sets the base delay between retries (doubled on every retry).
func WithBackoff(d time.Duration) Option { return func(s *Sender) { s.backoff = d } }

// New builds the sender from cfg: the service account is read from
// CredentialsFile or CredentialsJSON, ProjectID defaults to the project_id
// of the credentials.
func New(log *slog.Logger, cfg config.FCMConfig, opts ...Option) (*Sender, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	s := &Sender{
		log:        log.With(slog.String("component", "push.fcm")),
		client:     &http.Client{},
		url:        DefaultBaseURL,
		sem:        make(chan struct{}, cfg.Concurrency),
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		backoff:    defaultBackoff,
	}
	for _, opt := range opts {
		opt(s)
	}

	projectID := cfg.ProjectID
	if s.tokens == nil {
		creds, err := readCredentials(cfg)
		if err != nil {
			return nil, err
		}
		jwtCfg, err := google.JWTConfigFromJSON(creds, Scope)
		if err != nil {
			return nil, fmt.Errorf("fcm: parse service account: %w", err)
		}
		if projectID == "" {
			projectID, err = projectIDFromCredentials(creds)
			if err != nil {
				return nil, err
			}
		}
		// JWTConfig.TokenSource caches the access token until it expires.
		s.tokens = jwtCfg.TokenSource(context.Background())
	}
	if projectID == "" {
		return nil, errors.New("fcm: project id is empty (push.fcm.project-id / FCM_PROJECT_ID)")
	}
	s.url = s.url + "/v1/projects/" + projectID + "/messages:send"

	return s, nil
}

func readCredentials(cfg config.FCMConfig) ([]byte, error) {
	switch {
	case cfg.CredentialsJSON != "":
		return []byte(cfg.CredentialsJSON), nil
	case cfg.CredentialsFile != "":
		data, err := os.ReadFile(cfg.CredentialsFile)
		if err != nil {
			return nil, fmt.Errorf("fcm: read credentials file: %w", err)
		}
		return data, nil
	default:
		return nil, errors.New("fcm: credentials are not configured (push.fcm.credentials-file / credentials-json)")
	}
}

func projectIDFromCredentials(creds []byte) (string, error) {
	var sa struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(creds, &sa); err != nil {
		return "", fmt.Errorf("fcm: parse service account: %w", err)
	}
	if sa.ProjectID == "" {
		return "", errors.New("fcm: project id is neither configured nor present in the credentials")
	}
	return sa.ProjectID, nil
}

// FCM v1 message (https://firebase.google.com/docs/reference/fcm/rest/v1/projects.messages).
type request struct {
	Message message `json:"message"`
}

type message struct {
	Token        string            `json:"token"`
	Notification notification      `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *androidConfig    `json:"android,omitempty"`
	Webpush      *webpushConfig    `json:"webpush,omitempty"`
}

type notification struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
}

type androidConfig struct {
	Priority string `json:"priority"`
}

type webpushConfig struct {
	Headers      map[string]string `json:"headers,omitempty"`
	Notification notification      `json:"notification"`
}

func newMessage(device models.UserDevice, n models.Notification) message {
	note := notification{Title: n.Title, Body: n.Body}
	msg := message{Token: device.Token, Notification: note, Data: push.Data(n)}
	switch device.Platform {
	case models.PlatformWeb:
		msg.Webpush = &webpushConfig{Headers: map[string]string{"Urgency": "high"}, Notification: note}
	default:
		msg.Android = &androidConfig{Priority: "high"}
	}
	return msg
}

// errorResponse is the error envelope of Google APIs; FcmError details
// carry the FCM-specific code (UNREGISTERED, INVALID_ARGUMENT, ...).
type errorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type      string `json:"@type"`
			ErrorCode string `json:"errorCode"`
		} `json:"details"`
	} `json:"error"`
}

func (e errorResponse) fcmCode() string {
	for _, d := range e.Error.Details {
		if d.ErrorCode != "" {
			return d.ErrorCode
		}
	}
	return ""
}

// Send posts n to the device token, retrying 5xx/429 and transport errors
// up to MaxRetries times with exponential backoff. A token FCM reports as
// unregistered or invalid yields push.ErrInvalidToken; other 4xx are not
// retried.
func (s *Sender) Send(ctx context.Context, device models.UserDevice, n models.Notification) error {
	body, err := json.Marshal(request{Message: newMessage(device, n)})
	if err != nil {
		return fmt.Errorf("fcm: encode message: %w", err)
	}

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return fmt.Errorf("fcm: %w", ctx.Err())
	}

	for attempt := 0; ; attempt++ {
		retryAfter, retry, err := s.attempt(ctx, body)
		if err == nil {
			return nil
		}
		if !retry || attempt >= s.maxRetries {
			return err
		}

		delay := s.delay(attempt, retryAfter)
		s.log.Debug("fcm request failed, retrying",
			slog.Int("attempt", attempt+1), slog.Duration("delay", delay), slogger.Err(err))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return fmt.Errorf("fcm: %w", ctx.Err())
		}
	}
}

// attempt performs one request; retry reports whether the error is
// transient and retryAfter carries the server's Retry-After, if any.
func (s *Sender) attempt(ctx context.Context, body []byte) (retryAfter time.Duration, retry bool, err error) {
	token, err := s.tokens.Token()
	if err != nil {
		return 0, true, fmt.Errorf("fcm: access token: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return 0, false, fmt.Errorf("fcm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	token.SetAuthHeader(req)

	resp, err := s.client.Do(req)
	if err != nil {
		// The parent context is done: nothing to retry.
		if ctx.Err() != nil {
			return 0, false, fmt.Errorf("fcm: %w", ctx.Err())
		}
		return 0, true, fmt.Errorf("fcm: request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return 0, false, nil
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var apiErr errorResponse
	_ = json.Unmarshal(raw, &apiErr)
	code := apiErr.fcmCode()

	err = fmt.Errorf("fcm: status %d %s: %s", resp.StatusCode, code, apiErr.Error.Message)
	switch {
	case code == "UNREGISTERED", code == "INVALID_ARGUMENT", resp.StatusCode == http.StatusNotFound:
		return 0, false, fmt.Errorf("%w: %w", push.ErrInvalidToken, err)
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return parseRetryAfter(resp.Header.Get("Retry-After")), true, err
	default:
		return 0, false, err
	}
}

// delay returns the backoff before retry number attempt+1: base doubled on
// every attempt, or the server's Retry-After when it is larger; capped.
func (s *Sender) delay(attempt int, retryAfter time.Duration) time.Duration {
	d := s.backoff << uint(attempt)
	if d <= 0 {
		d = maxBackoff
	}
	return min(max(d, retryAfter), maxBackoff)
}

// parseRetryAfter reads a Retry-After in seconds; dates and garbage give 0.
func parseRetryAfter(header string) time.Duration {
	secs, err := strconv.Atoi(header)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
