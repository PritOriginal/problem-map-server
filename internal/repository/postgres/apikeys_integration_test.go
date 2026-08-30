//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/guregu/null/v6"
)

func (s *PostgresSuite) newAPIKey(owner int, raw string) models.APIKey {
	return models.APIKey{
		OwnerUserID:     owner,
		Name:            "key " + raw[len(raw)-4:],
		KeyHash:         models.HashAPIKey(raw),
		Prefix:          models.APIKeyDisplayPrefix(raw),
		Scopes:          []string{string(models.ScopeRead)},
		RateLimitPerMin: models.DefaultAPIKeyRateLimitPerMin,
		Active:          true,
	}
}

func (s *PostgresSuite) addAPIKey(k models.APIKey) models.APIKey {
	id, err := s.apiKeys.AddAPIKey(s.ctx, k)
	s.Require().NoError(err)
	stored, err := s.apiKeys.GetAPIKeyById(s.ctx, int(id))
	s.Require().NoError(err)
	return stored
}

func (s *PostgresSuite) TestAPIKeys_CRUD() {
	const raw = "pm_live_0123456789abcdef0123456789abcdef"
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	in := s.newAPIKey(fxUserAlice, raw)
	in.ExpiresAt = null.TimeFrom(expires)

	k := s.addAPIKey(in)
	s.Positive(k.ID)
	s.Equal(fxUserAlice, k.OwnerUserID)
	s.Equal(models.HashAPIKey(raw), k.KeyHash)
	s.Equal("pm_live_01234567", k.Prefix)
	s.Equal([]string{"read"}, k.Scopes)
	s.Equal(models.DefaultAPIKeyRateLimitPerMin, k.RateLimitPerMin)
	s.True(k.Active)
	s.False(k.LastUsedAt.Valid)
	s.True(k.ExpiresAt.Valid)
	s.True(expires.Equal(k.ExpiresAt.Time))
	s.WithinDuration(time.Now(), k.CreatedAt, time.Minute)

	s.Run("by hash", func() {
		got, err := s.apiKeys.GetAPIKeyByHash(s.ctx, models.HashAPIKey(raw))
		s.Require().NoError(err)
		s.Equal(k, got)

		_, err = s.apiKeys.GetAPIKeyByHash(s.ctx, models.HashAPIKey("pm_live_other"))
		s.ErrorIs(err, repository.ErrNotFound)
	})

	s.Run("duplicate hash", func() {
		_, err := s.apiKeys.AddAPIKey(s.ctx, s.newAPIKey(fxUserBob, raw))
		s.ErrorIs(err, repository.ErrExists)
	})

	s.Run("unknown owner", func() {
		_, err := s.apiKeys.AddAPIKey(s.ctx, s.newAPIKey(404, "pm_live_unknown_owner"))
		s.ErrorIs(err, repository.ErrInvalidReference)
	})

	s.Run("list", func() {
		second := s.addAPIKey(s.newAPIKey(fxUserAlice, "pm_live_second_key_of_alice"))
		bobs := s.addAPIKey(s.newAPIKey(fxUserBob, "pm_live_key_of_bob"))

		mine, err := s.apiKeys.GetAPIKeysByOwner(s.ctx, fxUserAlice)
		s.Require().NoError(err)
		s.Equal([]int{k.ID, second.ID}, ids(mine, func(k models.APIKey) int { return k.ID }))

		none, err := s.apiKeys.GetAPIKeysByOwner(s.ctx, 404)
		s.Require().NoError(err)
		s.Equal([]models.APIKey{}, none)

		all, err := s.apiKeys.GetAllAPIKeys(s.ctx)
		s.Require().NoError(err)
		s.Equal([]int{k.ID, second.ID, bobs.ID}, ids(all, func(k models.APIKey) int { return k.ID }))
	})

	s.Run("touch", func() {
		at := time.Now().UTC().Truncate(time.Second)
		s.Require().NoError(s.apiKeys.TouchAPIKey(s.ctx, k.ID, at))

		got, err := s.apiKeys.GetAPIKeyById(s.ctx, k.ID)
		s.Require().NoError(err)
		s.True(got.LastUsedAt.Valid)
		s.True(at.Equal(got.LastUsedAt.Time))
	})

	s.Run("revoke", func() {
		s.Require().NoError(s.apiKeys.RevokeAPIKey(s.ctx, k.ID))

		got, err := s.apiKeys.GetAPIKeyByHash(s.ctx, models.HashAPIKey(raw))
		s.Require().NoError(err)
		s.False(got.Active)

		// Revoking again is idempotent; a missing key is not found.
		s.NoError(s.apiKeys.RevokeAPIKey(s.ctx, k.ID))
		s.ErrorIs(s.apiKeys.RevokeAPIKey(s.ctx, 404), repository.ErrNotFound)
	})

	s.Run("owner deletion cascades", func() {
		var carol int
		s.Require().NoError(s.db.GetContext(s.ctx, &carol,
			"INSERT INTO users (name, login, password_hash) VALUES ('Carol', 'carol', 'hash-carol') RETURNING user_id"))
		s.addAPIKey(s.newAPIKey(carol, "pm_live_key_of_carol"))
		s.Equal(1, s.countRows("api_keys", "owner_user_id = $1", carol))

		_, err := s.db.ExecContext(s.ctx, "DELETE FROM users WHERE user_id = $1", carol)
		s.Require().NoError(err)
		s.Equal(0, s.countRows("api_keys", "owner_user_id = $1", carol))
	})
}
