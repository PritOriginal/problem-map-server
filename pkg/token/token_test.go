package token_test

import (
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateClaims(t *testing.T) {
	const key = "secret"

	t.Run("Ok", func(t *testing.T) {
		tok, err := token.CreateToken(time.Hour, 42, "admin", key)
		require.NoError(t, err)

		claims, err := token.ValidateClaims(tok, key)
		require.NoError(t, err)
		assert.Equal(t, token.Claims{UserID: 42, Role: "admin"}, claims)
	})

	t.Run("NoRole", func(t *testing.T) {
		tok, err := token.CreateToken(time.Hour, 1, "", key)
		require.NoError(t, err)

		claims, err := token.ValidateClaims(tok, key)
		require.NoError(t, err)
		assert.Equal(t, token.Claims{UserID: 1}, claims)
	})

	t.Run("WrongKey", func(t *testing.T) {
		tok, err := token.CreateToken(time.Hour, 1, "user", "other")
		require.NoError(t, err)

		_, err = token.ValidateClaims(tok, key)
		assert.Error(t, err)
	})

	t.Run("Expired", func(t *testing.T) {
		tok, err := token.CreateToken(-time.Hour, 1, "user", key)
		require.NoError(t, err)

		_, err = token.ValidateClaims(tok, key)
		assert.Error(t, err)
	})

	t.Run("AlgNone", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"sub": "1", "exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = token.ValidateClaims(tok, key)
		assert.Error(t, err)
	})

	t.Run("Garbage", func(t *testing.T) {
		_, err := token.ValidateClaims("not-a-jwt", key)
		assert.Error(t, err)
	})
}
