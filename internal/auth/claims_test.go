package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/auth"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type VerifySuite struct {
	suite.Suite
}

func TestVerify(t *testing.T) {
	suite.Run(t, new(VerifySuite))
}

func (suite *VerifySuite) TestVerify() {
	errStore := errors.New("redis down")
	access := func(role string, version int64) token.Claims {
		return token.Claims{UserID: 7, Role: role, Type: token.TypeAccess, Version: version}
	}

	tests := []struct {
		name       string
		claims     token.Claims
		noChecker  bool
		version    *int64
		versionErr error
		want       auth.Identity
		wantErr    error
	}{
		{name: "Ok", claims: access("admin", 2), version: ptr(int64(2)), want: auth.Identity{UserID: 7, Role: models.RoleAdmin}},
		{name: "OkRoleDefaultsToUser", claims: access("", 0), version: ptr(int64(0)), want: auth.Identity{UserID: 7, Role: models.RoleUser}},
		{name: "OkWithoutChecker", claims: access("moderator", 5), noChecker: true, want: auth.Identity{UserID: 7, Role: models.RoleModerator}},
		{name: "OkStoreUnavailableFailsOpen", claims: access("user", 1), versionErr: errStore, want: auth.Identity{UserID: 7, Role: models.RoleUser}},
		{name: "ErrVersionMismatch", claims: access("user", 1), version: ptr(int64(2)), wantErr: auth.ErrRevoked},
		{name: "ErrRefreshToken", claims: token.Claims{UserID: 7, Type: token.TypeRefresh, Version: 2}, wantErr: auth.ErrNotAccessToken},
		{name: "ErrLegacyNoType", claims: token.Claims{UserID: 7, Version: 2}, wantErr: auth.ErrNotAccessToken},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			var checker auth.VersionChecker
			if !tt.noChecker {
				m := auth.NewMockVersionChecker(suite.T())
				if tt.version != nil || tt.versionErr != nil {
					var v int64
					if tt.version != nil {
						v = *tt.version
					}
					m.On("AuthVersion", mock.Anything, 7).Once().Return(v, tt.versionErr)
				}
				checker = m
			}

			got, err := auth.Verify(context.Background(), slogdiscard.NewDiscardLogger(), tt.claims, checker)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(tt.want, got)
		})
	}
}

func ptr[T any](v T) *T { return &v }
