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
	body := []byte(`{"event_id":"e1"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	suite.Equal(want, webhooks.Sign(secret, body))
	// Deterministic and secret-dependent.
	suite.Equal(webhooks.Sign(secret, body), webhooks.Sign(secret, body))
	suite.NotEqual(webhooks.Sign("other", body), webhooks.Sign(secret, body))
}

func (suite *SignSuite) TestVerifySignature() {
	const secret = "s3cr3t"
	body := []byte(`{"event_id":"e1"}`)
	valid := webhooks.Sign(secret, body)

	tests := []struct {
		name      string
		secret    string
		body      []byte
		signature string
		want      bool
	}{
		{name: "Valid", secret: secret, body: body, signature: valid, want: true},
		{name: "WrongSecret", secret: "nope", body: body, signature: valid},
		{name: "TamperedBody", secret: secret, body: []byte(`{"event_id":"e2"}`), signature: valid},
		{name: "MissingPrefix", secret: secret, body: body, signature: valid[len("sha256="):]},
		{name: "NotHex", secret: secret, body: body, signature: "sha256=zz"},
		{name: "Empty", secret: secret, body: body, signature: ""},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.want, webhooks.VerifySignature(tt.secret, tt.body, tt.signature))
		})
	}
}
