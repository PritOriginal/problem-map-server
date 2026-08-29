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

// Sign returns the X-Signature value of a delivery: "sha256=" followed by
// the lower-case hex HMAC-SHA256, under secret, of the X-Timestamp value,
// a dot and the body ("<timestamp>.<body>"). Signing the timestamp lets a
// receiver reject replayed deliveries by their age.
func Sign(secret, timestamp string, body []byte) string {
	return SignaturePrefix + hex.EncodeToString(signedDigest(secret, timestamp, body))
}

// VerifySignature reports whether signature (an X-Signature value) is the
// valid signature of body sent with timestamp (the X-Timestamp value)
// under secret. The comparison is constant-time. The caller should also
// check that timestamp is recent (see the README).
func VerifySignature(secret, timestamp string, body []byte, signature string) bool {
	if !strings.HasPrefix(signature, SignaturePrefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(signature, SignaturePrefix))
	if err != nil {
		return false
	}
	return hmac.Equal(signedDigest(secret, timestamp, body), want)
}

func signedDigest(secret, timestamp string, body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return mac.Sum(nil)
}
