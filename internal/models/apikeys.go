package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	"github.com/guregu/null/v6"
)

// APIKeyScope names what an API key may do. Only ScopeRead is honoured by
// the server today: ScopeWrite is reserved and requests writing through a
// key are refused regardless of the scopes.
type APIKeyScope string

const (
	ScopeRead  APIKeyScope = "read"
	ScopeWrite APIKeyScope = "write"
)

const (
	// APIKeyPrefix opens every issued key: "pm_live_" + APIKeySecretHex hex chars.
	APIKeyPrefix = "pm_live_"
	// APIKeySecretBytes is the entropy of the key (hex-encoded to twice as
	// many characters).
	APIKeySecretBytes = 16
	// APIKeyDisplayLen is how many characters of the key are kept in
	// APIKey.Prefix for display: "pm_live_" plus 8 hex chars.
	APIKeyDisplayLen = len(APIKeyPrefix) + 8
	// DefaultAPIKeyRateLimitPerMin is the per-key request quota of a new key.
	DefaultAPIKeyRateLimitPerMin = 600
	// MaxAPIKeyNameLen bounds the name of a key.
	MaxAPIKeyNameLen = 64
)

// APIKey is a read-only credential of the open-data API. The key itself is
// shown once when created; only its SHA-256 (KeyHash) and the displayable
// head (Prefix) are stored.
type APIKey struct {
	ID          int    `json:"api_key_id" db:"api_key_id"`
	OwnerUserID int    `json:"owner_user_id" db:"owner_user_id"`
	Name        string `json:"name" db:"name"`
	KeyHash     string `json:"-" db:"key_hash"`
	Prefix      string `json:"prefix" db:"prefix"`
	// Scopes lists the APIKeyScope values granted to the key.
	Scopes          []string  `json:"scopes" db:"-"`
	RateLimitPerMin int       `json:"rate_limit_per_min" db:"rate_limit_per_min"`
	Active          bool      `json:"active" db:"active"`
	LastUsedAt      null.Time `json:"last_used_at" db:"last_used_at" swaggertype:"string" format:"date-time"`
	ExpiresAt       null.Time `json:"expires_at" db:"expires_at" swaggertype:"string" format:"date-time"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// HasScope reports whether the key carries the scope.
func (k APIKey) HasScope(scope APIKeyScope) bool {
	return slices.Contains(k.Scopes, string(scope))
}

// Expired reports whether the key's expiry has passed at now.
func (k APIKey) Expired(now time.Time) bool {
	return k.ExpiresAt.Valid && !now.Before(k.ExpiresAt.Time)
}

// Identity is what the request carries once the key was verified.
func (k APIKey) Identity() ApiKeyIdentity {
	return ApiKeyIdentity{KeyID: k.ID, OwnerID: k.OwnerUserID, Prefix: k.Prefix, Scopes: k.Scopes, RateLimitPerMin: k.RateLimitPerMin}
}

// HashAPIKey returns the hex SHA-256 of a raw key, the form stored in
// APIKey.KeyHash.
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// APIKeyDisplayPrefix returns the head of the raw key kept for display.
func APIKeyDisplayPrefix(raw string) string {
	if len(raw) < APIKeyDisplayLen {
		return raw
	}
	return raw[:APIKeyDisplayLen]
}

// ApiKeyIdentity identifies the API key a request was authenticated with.
type ApiKeyIdentity struct {
	KeyID   int
	OwnerID int
	// Prefix is the displayable head of the key (metrics label).
	Prefix          string
	Scopes          []string
	RateLimitPerMin int
}

type apiKeyCtxKey struct{}

// ContextWithAPIKey records the verified API key of the request.
func ContextWithAPIKey(ctx context.Context, id ApiKeyIdentity) context.Context {
	return context.WithValue(ctx, apiKeyCtxKey{}, id)
}

// APIKeyFromContext returns the API key the request was authenticated with.
func APIKeyFromContext(ctx context.Context) (ApiKeyIdentity, bool) {
	id, ok := ctx.Value(apiKeyCtxKey{}).(ApiKeyIdentity)
	return id, ok
}
