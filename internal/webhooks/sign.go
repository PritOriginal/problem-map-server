// Package webhooks implements the outgoing side of webhooks: payload
// signing, the SSRF guard on target URLs and the HTTP sender used by the
// delivery use case.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SignaturePrefix precedes the hex digest in the X-Signature header.
const SignaturePrefix = "sha256="

// Sign returns the X-Signature value of body: "sha256=" followed by the
// lower-case hex HMAC-SHA256 of body under secret.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature reports whether signature (an X-Signature value) is the
// valid signature of body under secret. The comparison is constant-time.
func VerifySignature(secret string, body []byte, signature string) bool {
	if !strings.HasPrefix(signature, SignaturePrefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(signature, SignaturePrefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}
