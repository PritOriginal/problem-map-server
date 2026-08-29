package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/assert"
)

func TestAPIKey_Helpers(t *testing.T) {
	const raw = "pm_live_0123456789abcdef0123456789abcdef"

	assert.Equal(t, "pm_live_01234567", models.APIKeyDisplayPrefix(raw))
	assert.Equal(t, "pm_", models.APIKeyDisplayPrefix("pm_"))
	assert.Len(t, models.HashAPIKey(raw), 64)
	assert.NotEqual(t, models.HashAPIKey(raw), models.HashAPIKey(raw+"x"))

	now := time.Now()
	assert.False(t, models.APIKey{}.Expired(now))
	assert.False(t, models.APIKey{ExpiresAt: null.TimeFrom(now.Add(time.Second))}.Expired(now))
	assert.True(t, models.APIKey{ExpiresAt: null.TimeFrom(now)}.Expired(now))
	assert.True(t, models.APIKey{ExpiresAt: null.TimeFrom(now.Add(-time.Second))}.Expired(now))

	k := models.APIKey{ID: 5, OwnerUserID: 7, Prefix: "pm_live_01234567", Scopes: []string{"read"}, RateLimitPerMin: 600}
	assert.True(t, k.HasScope(models.ScopeRead))
	assert.False(t, k.HasScope(models.ScopeWrite))
	assert.Equal(t, models.ApiKeyIdentity{KeyID: 5, OwnerID: 7, Prefix: "pm_live_01234567", Scopes: []string{"read"}, RateLimitPerMin: 600}, k.Identity())
}

func TestAPIKeyContext(t *testing.T) {
	_, ok := models.APIKeyFromContext(context.Background())
	assert.False(t, ok)

	ctx := models.ContextWithAPIKey(context.Background(), models.ApiKeyIdentity{KeyID: 5})
	got, ok := models.APIKeyFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, 5, got.KeyID)
}
