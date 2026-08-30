package idempotency_test

import (
	"context"
	"errors"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/idempotency"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
)

var itemStoreKey = idempotency.ItemKey(testUserID, idemKey)

func (s *IdempotencySuite) keys(store idempotency.Store) *idempotency.Keys {
	return idempotency.NewKeys(slogdiscard.NewDiscardLogger(), store, idempotency.Config{TTL: time.Hour})
}

// TestKeysReserveFresh: an unused key is reserved and the item is applied.
func (s *IdempotencySuite) TestKeysReserveFresh() {
	store := idempotency.NewMockStore(s.T())
	store.On("SetNX", mock.Anything, itemStoreKey, mock.Anything, time.Hour).Once().Return(true, nil)

	markID, replayed, ok := s.keys(store).Reserve(context.Background(), testUserID, idemKey)

	s.Zero(markID)
	s.False(replayed)
	s.True(ok)
}

// TestKeysReplay: a key that already created a mark replays its id instead
// of creating a second one.
func (s *IdempotencySuite) TestKeysReplay() {
	store := idempotency.NewMockStore(s.T())
	store.On("SetNX", mock.Anything, itemStoreKey, mock.Anything, time.Hour).Once().Return(false, nil)
	store.On("GetBytes", mock.Anything, itemStoreKey).Once().Return([]byte(`{"mark_id":42}`), nil)

	markID, replayed, ok := s.keys(store).Reserve(context.Background(), testUserID, idemKey)

	s.Equal(int64(42), markID)
	s.True(replayed)
	s.True(ok)
}

// TestKeysReserveNotUsable: a key that cannot be used for idempotency lets
// the item through unchanged (fail open).
func (s *IdempotencySuite) TestKeysReserveNotUsable() {
	tests := []struct {
		name  string
		key   string
		setup func(*idempotency.MockStore)
	}{
		{name: "MalformedKey", key: "not-a-uuid"},
		{name: "EmptyKey", key: ""},
		{
			name: "StoreDown",
			key:  idemKey,
			setup: func(store *idempotency.MockStore) {
				store.On("SetNX", mock.Anything, itemStoreKey, mock.Anything, time.Hour).Once().
					Return(false, errors.New("redis is down"))
			},
		},
		{
			name: "StillReserved",
			key:  idemKey,
			setup: func(store *idempotency.MockStore) {
				store.On("SetNX", mock.Anything, itemStoreKey, mock.Anything, time.Hour).Once().Return(false, nil)
				store.On("GetBytes", mock.Anything, itemStoreKey).Once().Return([]byte(`{}`), nil)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			store := idempotency.NewMockStore(s.T())
			if tt.setup != nil {
				tt.setup(store)
			}

			markID, replayed, ok := s.keys(store).Reserve(context.Background(), testUserID, tt.key)

			s.Zero(markID)
			s.False(replayed)
			s.False(ok)
		})
	}
}

// TestKeysCommitAndRelease: a created mark is stored under the key, a
// rejected item frees it.
func (s *IdempotencySuite) TestKeysCommitAndRelease() {
	store := idempotency.NewMockStore(s.T())
	store.On("Set", mock.Anything, itemStoreKey, mock.Anything, time.Hour).Once().Return(nil)
	store.On("Del", mock.Anything, itemStoreKey).Once().Return(nil)

	keys := s.keys(store)
	keys.Commit(context.Background(), testUserID, idemKey, 42)
	// A commit without an id stores nothing.
	keys.Commit(context.Background(), testUserID, idemKey, 0)
	keys.Release(context.Background(), testUserID, idemKey)
}

// TestKeysNil: a nil *Keys and a nil store are usable and simply report
// "no idempotency", so a batch works without Redis.
func (s *IdempotencySuite) TestKeysNil() {
	var nilKeys *idempotency.Keys
	for _, keys := range []*idempotency.Keys{nilKeys, s.keys(nil)} {
		markID, replayed, ok := keys.Reserve(context.Background(), testUserID, idemKey)
		s.Zero(markID)
		s.False(replayed)
		s.False(ok)
		keys.Commit(context.Background(), testUserID, idemKey, 42)
		keys.Release(context.Background(), testUserID, idemKey)
	}
}
