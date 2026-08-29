package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository/redis"
	"github.com/stretchr/testify/suite"
)

// NilSafeSuite checks that a missing client (nil *Redis or a *Redis without a
// client) never panics: every operation reports ErrUnavailable so callers
// can fail open.
type NilSafeSuite struct {
	suite.Suite
}

func TestNilSafe(t *testing.T) {
	suite.Run(t, new(NilSafeSuite))
}

func (s *NilSafeSuite) TestOperations() {
	ctx := context.Background()

	tests := []struct {
		name string
		repo *redis.Redis
	}{
		{name: "NilPointer", repo: nil},
		{name: "NoClient", repo: &redis.Redis{}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			r := tt.repo

			s.ErrorIs(r.Ping(ctx), redis.ErrUnavailable)
			s.NoError(r.Close())
			s.False(r.Exists(ctx, "k"))

			var v any
			s.ErrorIs(r.Get(ctx, "k", &v), redis.ErrUnavailable)
			_, err := r.GetBytes(ctx, "k")
			s.ErrorIs(err, redis.ErrUnavailable)
			s.ErrorIs(r.Set(ctx, "k", "v", time.Minute), redis.ErrUnavailable)
			_, err = r.SetNX(ctx, "k", "v", time.Minute)
			s.ErrorIs(err, redis.ErrUnavailable)
			s.ErrorIs(r.Del(ctx, "k"), redis.ErrUnavailable)

			_, _, err = r.Incr(ctx, "k", time.Minute)
			s.ErrorIs(err, redis.ErrUnavailable)

			s.ErrorIs(r.SaveRefresh(ctx, 1, "jti", time.Minute), redis.ErrUnavailable)
			_, err = r.DeleteRefresh(ctx, 1, "jti")
			s.ErrorIs(err, redis.ErrUnavailable)
			s.ErrorIs(r.DeleteAllRefresh(ctx, 1), redis.ErrUnavailable)

			_, err = r.AuthVersion(ctx, 1)
			s.ErrorIs(err, redis.ErrUnavailable)
			_, err = r.IncrAuthVersion(ctx, 1)
			s.ErrorIs(err, redis.ErrUnavailable)
		})
	}
}

// TestNew_UnreachableReturnsClient: a failed ping is reported, but the client
// is still returned so the app can start and fail open until Redis is back.
func (s *NilSafeSuite) TestNew_UnreachableReturnsClient() {
	r, err := redis.New(config.RedisConfig{Host: "127.0.0.1", Port: 1})
	s.Error(err)
	s.Require().NotNil(r)
	s.NotNil(r.Client)

	s.Error(r.Ping(context.Background()))
	_, err = r.AuthVersion(context.Background(), 1)
	s.Error(err)
	s.NoError(r.Close())
}
