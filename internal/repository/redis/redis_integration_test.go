//go:build integration

package redis_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/repository/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

const redisImage = "redis:7-alpine"

type RedisSuite struct {
	suite.Suite

	ctx  context.Context
	repo *redis.Redis
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func (s *RedisSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := tcredis.Run(s.ctx, redisImage)
	s.Require().NoError(err, "start redis container")
	testcontainers.CleanupContainer(s.T(), container)

	host, err := container.Host(s.ctx)
	s.Require().NoError(err)
	port, err := container.MappedPort(s.ctx, "6379/tcp")
	s.Require().NoError(err)

	s.repo, err = redis.New(config.RedisConfig{Host: host, Port: int(port.Num())})
	s.Require().NoError(err, "connect via repository constructor")
}

func (s *RedisSuite) TearDownSuite() {
	if s.repo != nil {
		_ = s.repo.Close()
	}
}

func (s *RedisSuite) SetupTest() {
	s.Require().NoError(s.repo.Client.FlushDB(s.ctx).Err())
}

func (s *RedisSuite) TestNew_BadAddress() {
	_, err := redis.New(config.RedisConfig{Host: "127.0.0.1", Port: 1})
	s.Error(err)
	s.ErrorContains(err, "storage.redis.New")
}

func (s *RedisSuite) TestPing() {
	s.NoError(s.repo.Ping(s.ctx))
}

func (s *RedisSuite) TestSetGet() {
	type payload struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	want := payload{ID: 7, Name: "seven"}
	raw, err := json.Marshal(want)
	s.Require().NoError(err)

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "json bytes", key: "obj:bytes", value: raw},
		{name: "json string", key: "obj:string", value: string(raw)},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().NoError(s.repo.Set(s.ctx, tt.key, tt.value, 0))

			var got payload
			s.Require().NoError(s.repo.Get(s.ctx, tt.key, &got))
			s.Equal(want, got)

			b, err := s.repo.GetBytes(s.ctx, tt.key)
			s.Require().NoError(err)
			s.JSONEq(string(raw), string(b))

			ttl, err := s.repo.Client.TTL(s.ctx, tt.key).Result()
			s.Require().NoError(err)
			s.Equal(time.Duration(-1), ttl, "no expiration must be set")
		})
	}
}

func (s *RedisSuite) TestSet_WithExpiration() {
	s.Require().NoError(s.repo.Set(s.ctx, "ttl", "v", 10*time.Second))

	ttl, err := s.repo.Client.TTL(s.ctx, "ttl").Result()
	s.Require().NoError(err)
	s.Greater(ttl, 5*time.Second)
	s.LessOrEqual(ttl, 10*time.Second)
}

func (s *RedisSuite) TestSet_Overwrite() {
	s.Require().NoError(s.repo.Set(s.ctx, "k", "1", 0))
	s.Require().NoError(s.repo.Set(s.ctx, "k", "2", 0))

	b, err := s.repo.GetBytes(s.ctx, "k")
	s.Require().NoError(err)
	s.Equal("2", string(b))
}

func (s *RedisSuite) TestGet_MissingKey() {
	var v map[string]any
	s.ErrorIs(s.repo.Get(s.ctx, "missing", &v), goredis.Nil)
}

func (s *RedisSuite) TestGet_InvalidJSON() {
	s.Require().NoError(s.repo.Set(s.ctx, "not-json", "{oops", 0))

	var v map[string]any
	s.ErrorContains(s.repo.Get(s.ctx, "not-json", &v), "invalid character")
}

func (s *RedisSuite) TestExists() {
	s.Require().NoError(s.repo.Set(s.ctx, "present", "x", 0))

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "existing key", key: "present", want: true},
		{name: "missing key", key: "absent", want: false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.want, s.repo.Exists(s.ctx, tt.key))
		})
	}
}

func (s *RedisSuite) TestExists_AfterExpiration() {
	s.Require().NoError(s.repo.Set(s.ctx, "short", "x", 50*time.Millisecond))
	s.True(s.repo.Exists(s.ctx, "short"))

	s.Eventually(func() bool { return !s.repo.Exists(s.ctx, "short") }, 2*time.Second, 10*time.Millisecond)
}

func (s *RedisSuite) TestIncr() {
	const key = "counter"

	n, err := s.repo.Incr(s.ctx, key, 30*time.Second)
	s.Require().NoError(err)
	s.Equal(int64(1), n)

	ttlFirst, err := s.repo.Client.TTL(s.ctx, key).Result()
	s.Require().NoError(err)
	s.Greater(ttlFirst, 25*time.Second, "expiration is set on creation")
	s.LessOrEqual(ttlFirst, 30*time.Second)

	// EXPIRE NX must not touch the TTL of an existing key, even with a
	// different expiration.
	n, err = s.repo.Incr(s.ctx, key, time.Hour)
	s.Require().NoError(err)
	s.Equal(int64(2), n)

	ttlSecond, err := s.repo.Client.TTL(s.ctx, key).Result()
	s.Require().NoError(err)
	s.LessOrEqual(ttlSecond, ttlFirst)
	s.Greater(ttlSecond, 25*time.Second)

	n, err = s.repo.Incr(s.ctx, key, time.Hour)
	s.Require().NoError(err)
	s.Equal(int64(3), n)
}

func (s *RedisSuite) TestIncr_NonIntegerValue() {
	s.Require().NoError(s.repo.Set(s.ctx, "text", "abc", 0))

	_, err := s.repo.Incr(s.ctx, "text", time.Minute)
	s.Error(err)
}

func (s *RedisSuite) TestIncr_IndependentKeys() {
	for i := 1; i <= 3; i++ {
		n, err := s.repo.Incr(s.ctx, "a", time.Minute)
		s.Require().NoError(err)
		s.Equal(int64(i), n)
	}
	n, err := s.repo.Incr(s.ctx, "b", time.Minute)
	s.Require().NoError(err)
	s.Equal(int64(1), n)
}
