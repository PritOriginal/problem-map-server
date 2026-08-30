package webhooks_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/webhooks"
	"github.com/stretchr/testify/suite"
)

type SignSuite struct {
	suite.Suite
}

func TestSign(t *testing.T) {
	suite.Run(t, new(SignSuite))
}

func (suite *SignSuite) TestSign() {
	const secret = "s3cr3t"
	const timestamp = "1700000000"
	body := []byte(`{"event_id":"e1"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	suite.Equal(want, webhooks.Sign(secret, timestamp, body))
	// Deterministic, secret- and timestamp-dependent.
	suite.Equal(webhooks.Sign(secret, timestamp, body), webhooks.Sign(secret, timestamp, body))
	suite.NotEqual(webhooks.Sign("other", timestamp, body), webhooks.Sign(secret, timestamp, body))
	suite.NotEqual(webhooks.Sign(secret, "1700000001", body), webhooks.Sign(secret, timestamp, body))
}

func (suite *SignSuite) TestVerifySignature() {
	const secret = "s3cr3t"
	const timestamp = "1700000000"
	body := []byte(`{"event_id":"e1"}`)
	valid := webhooks.Sign(secret, timestamp, body)

	tests := []struct {
		name      string
		secret    string
		timestamp string
		body      []byte
		signature string
		want      bool
	}{
		{name: "Valid", secret: secret, timestamp: timestamp, body: body, signature: valid, want: true},
		{name: "WrongSecret", secret: "nope", timestamp: timestamp, body: body, signature: valid},
		{name: "TamperedBody", secret: secret, timestamp: timestamp, body: []byte(`{"event_id":"e2"}`), signature: valid},
		{name: "TamperedTimestamp", secret: secret, timestamp: "1700000001", body: body, signature: valid},
		{name: "MissingPrefix", secret: secret, timestamp: timestamp, body: body, signature: valid[len("sha256="):]},
		{name: "NotHex", secret: secret, timestamp: timestamp, body: body, signature: "sha256=zz"},
		{name: "Empty", secret: secret, timestamp: timestamp, body: body, signature: ""},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.want, webhooks.VerifySignature(tt.secret, tt.timestamp, tt.body, tt.signature))
		})
	}
}
