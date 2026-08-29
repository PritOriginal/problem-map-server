package webhooks_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/webhooks"
	"github.com/stretchr/testify/suite"
)

type SenderSuite struct {
	suite.Suite
}

func TestSender(t *testing.T) {
	suite.Run(t, new(SenderSuite))
}

// received is what the test receiver saw in the last request.
type received struct {
	headers http.Header
	body    []byte
}

// newReceiver starts a TLS server answering with status and records the
// request.
func (suite *SenderSuite) newReceiver(status int, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *atomic.Pointer[received]) {
	var last atomic.Pointer[received]
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		last.Store(&received{headers: r.Header.Clone(), body: body})
		if handler != nil {
			handler(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "thanks")
	}))
	return srv, &last
}

// newSender builds a sender trusting the httptest certificate. The policy
// allows private addresses because the test receiver is on loopback.
func (suite *SenderSuite) newSender(srv *httptest.Server, timeout time.Duration) *webhooks.Sender {
	return webhooks.NewSender(webhooks.SenderOptions{
		Timeout:   timeout,
		Policy:    webhooks.URLPolicy{AllowPrivate: true},
		Transport: srv.Client().Transport,
	})
}

func (suite *SenderSuite) TestSendOK() {
	srv, last := suite.newReceiver(http.StatusOK, nil)
	defer srv.Close()

	body := []byte(`{"event_id":"e1","subject":"mark.status_changed"}`)
	res := suite.newSender(srv, time.Second).Send(context.Background(), webhooks.Request{
		WebhookID: 42,
		URL:       srv.URL + "/hook",
		Secret:    "s3cr3t",
		EventID:   "e1",
		Body:      body,
	})

	suite.True(res.OK(), "err: %v", res.Err)
	suite.Equal(http.StatusOK, res.StatusCode)

	got := last.Load()
	suite.Require().NotNil(got)
	suite.Equal(body, got.body)
	suite.Equal("application/json", got.headers.Get("Content-Type"))
	suite.Equal("42", got.headers.Get(webhooks.HeaderWebhookID))
	suite.Equal("e1", got.headers.Get(webhooks.HeaderEventID))
	timestamp := got.headers.Get(webhooks.HeaderTimestamp)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	suite.Require().NoError(err)
	suite.InDelta(time.Now().Unix(), ts, 5)
	suite.Equal(webhooks.Sign("s3cr3t", timestamp, body), got.headers.Get(webhooks.HeaderSignature))
	suite.True(webhooks.VerifySignature("s3cr3t", timestamp, got.body, got.headers.Get(webhooks.HeaderSignature)))
	suite.False(webhooks.VerifySignature("s3cr3t", "0", got.body, got.headers.Get(webhooks.HeaderSignature)), "timestamp is signed")
}

func (suite *SenderSuite) TestSendStatuses() {
	tests := []struct {
		name   string
		status int
		ok     bool
	}{
		{name: "Created", status: http.StatusCreated, ok: true},
		{name: "NoContent", status: http.StatusNoContent, ok: true},
		{name: "BadRequest", status: http.StatusBadRequest},
		{name: "Unauthorized", status: http.StatusUnauthorized},
		{name: "ServerError", status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			srv, _ := suite.newReceiver(tt.status, nil)
			defer srv.Close()

			res := suite.newSender(srv, time.Second).Send(context.Background(), webhooks.Request{URL: srv.URL, Body: []byte("{}")})
			suite.Equal(tt.ok, res.OK())
			suite.Equal(tt.status, res.StatusCode)
			if !tt.ok {
				suite.ErrorContains(res.Err, strconv.Itoa(tt.status))
				suite.ErrorContains(res.Err, "thanks")
			}
		})
	}
}

func (suite *SenderSuite) TestSendDoesNotFollowRedirect() {
	var followed atomic.Bool
	srv, _ := suite.newReceiver(0, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			followed.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	defer srv.Close()

	res := suite.newSender(srv, time.Second).Send(context.Background(), webhooks.Request{URL: srv.URL + "/hook", Body: []byte("{}")})
	suite.False(res.OK())
	suite.ErrorIs(res.Err, webhooks.ErrRedirect)
	suite.Equal(http.StatusFound, res.StatusCode)
	suite.False(followed.Load(), "redirect target must not be requested")
}

func (suite *SenderSuite) TestSendTimeout() {
	release := make(chan struct{})
	srv, _ := suite.newReceiver(0, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	defer close(release)

	start := time.Now()
	res := suite.newSender(srv, 200*time.Millisecond).Send(context.Background(), webhooks.Request{URL: srv.URL, Body: []byte("{}")})
	suite.False(res.OK())
	suite.Zero(res.StatusCode)
	suite.Less(time.Since(start), 5*time.Second)
}

func (suite *SenderSuite) TestSendRejectsForbiddenTarget() {
	sender := webhooks.NewSender(webhooks.SenderOptions{Timeout: time.Second})

	tests := []struct {
		name string
		url  string
	}{
		{name: "Http", url: "http://example.org/hook"},
		{name: "Loopback", url: "https://127.0.0.1/hook"},
		{name: "Private", url: "https://10.0.0.1/hook"},
		{name: "Localhost", url: "https://localhost/hook"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			res := sender.Send(context.Background(), webhooks.Request{URL: tt.url, Body: []byte("{}")})
			suite.ErrorIs(res.Err, webhooks.ErrForbiddenTarget)
			suite.Zero(res.StatusCode)
		})
	}
}
