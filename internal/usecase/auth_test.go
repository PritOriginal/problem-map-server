package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"

	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type method[T any] struct {
	data T
	err  error
}

// errRepo stands for an unexpected repository failure that must be wrapped
// and passed through the usecase as is.
var errRepo = errors.New("repository error")

// assertRepoErr checks that the usecase mapped the first non-nil repository
// error correctly: repository.ErrNotFound -> usecase.ErrNotFound,
// repository.ErrExists -> usecase.ErrConflict, anything else is wrapped.
func assertRepoErr(s *suite.Suite, got error, repoErrs ...error) {
	s.T().Helper()

	for _, repoErr := range repoErrs {
		if repoErr == nil {
			continue
		}
		switch {
		case errors.Is(repoErr, repository.ErrNotFound):
			s.ErrorIs(got, usecase.ErrNotFound)
		case errors.Is(repoErr, repository.ErrExists):
			s.ErrorIs(got, usecase.ErrConflict)
		case errors.Is(repoErr, repository.ErrInvalidReference):
			s.ErrorIs(got, usecase.ErrInvalidArgument)
		default:
			s.ErrorIs(got, repoErr)
		}
		return
	}
	s.Fail("assertRepoErr: no repository error given")
}

type AuthSuite struct {
	suite.Suite
	uc        *usecase.Auth
	log       *slog.Logger
	usersRepo *usecase.MockUsersRepository
	refresh   *usecase.MockRefreshStore
	versions  *usecase.MockAuthVersionStore
	authCfg   config.AuthConfig
}

func (suite *AuthSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.usersRepo = usecase.NewMockUsersRepository(suite.T())
	suite.refresh = usecase.NewMockRefreshStore(suite.T())
	suite.versions = usecase.NewMockAuthVersionStore(suite.T())
	cfg := config.MustLoadPath("../../configs/config-tests.yaml")
	suite.authCfg = cfg.Auth
	suite.uc = usecase.NewAuth(suite.log, cfg.Auth, usecase.AuthRepositories{
		Users:         suite.usersRepo,
		RefreshTokens: suite.refresh,
		AuthVersions:  suite.versions,
	})
}

// errStore stands for an unavailable Redis: every store call fails open.
var errStore = errors.New("store unavailable")

// expectIssue registers the store calls made when a token pair is issued:
// the version lookup and the refresh id registration.
func (suite *AuthSuite) expectIssue(userID int, version int64, versionErr, saveErr error) {
	suite.versions.On("AuthVersion", mock.Anything, userID).Once().Return(version, versionErr)
	suite.refresh.On("SaveRefresh", mock.Anything, userID, mock.AnythingOfType("string"), suite.authCfg.JWT.Refresh.ExpiredIn).
		Once().Return(saveErr)
}

// parseClaims verifies a token with the key and returns its claims.
func (suite *AuthSuite) parseClaims(tokenString, key string) token.Claims {
	claims, err := token.ValidateClaims(tokenString, key)
	suite.Require().NoError(err)
	return claims
}

// refreshToken issues a refresh token the way the usecase does.
// refreshToken issues a refresh token at auth version currentVersion.
func (suite *AuthSuite) refreshToken(userID int, jti string) string {
	return suite.refreshTokenV(userID, jti, currentVersion)
}

func (suite *AuthSuite) refreshTokenV(userID int, jti string, version int64) string {
	tok, err := token.Create(token.Params{
		TTL: suite.authCfg.JWT.Refresh.ExpiredIn, UserID: userID, Role: string(models.RoleUser),
		Type: token.TypeRefresh, ID: jti, Version: version,
	}, suite.authCfg.JWT.Refresh.Key)
	suite.Require().NoError(err)
	return tok
}

// currentVersion is the auth version the stores report in the tests.
const currentVersion int64 = 2

func TestAuth(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (suite *AuthSuite) TestSignUp() {
	tests := []struct {
		name           string
		getUserByLogin method[models.User]
		addUser        method[int64]
	}{
		{
			name: "Ok",
			getUserByLogin: method[models.User]{
				err: repository.ErrNotFound,
			},
			addUser: method[int64]{
				err: nil,
			},
		},
		{
			name: "ErrGetUserByLogin",
			getUserByLogin: method[models.User]{
				err: errRepo,
			},
			addUser: method[int64]{
				err: nil,
			},
		},
		{
			name: "ErrAddUser",
			getUserByLogin: method[models.User]{
				err: repository.ErrNotFound,
			},
			addUser: method[int64]{
				err: errRepo,
			},
		},
		{
			name: "ErrConflictAddUserExists",
			getUserByLogin: method[models.User]{
				err: repository.ErrNotFound,
			},
			addUser: method[int64]{
				err: repository.ErrExists,
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.usersRepo.On("GetUserByLogin", mock.Anything, mock.AnythingOfType("string")).Once().
					Return(tt.getUserByLogin.data, tt.getUserByLogin.err)
				if !errors.Is(tt.getUserByLogin.err, repository.ErrNotFound) {
					return
				}

				suite.usersRepo.On("AddUser", mock.Anything, mock.Anything).Once().
					Return(tt.addUser.data, tt.addUser.err)
				if tt.addUser.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.SignUp(context.Background(), usecase.SignUpParams{
				Username:  "username",
				Login:     "login",
				Password:  "password",
				HomePoint: &models.Point{},
			})

			switch {
			case errors.Is(tt.getUserByLogin.err, repository.ErrNotFound) && tt.addUser.err == nil:
				suite.NoError(gotErr)
			default:
				assertRepoErr(&suite.Suite, gotErr, tt.addUser.err, tt.getUserByLogin.err)
			}

			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *AuthSuite) TestSignIn() {
	password := "password"
	passwordHash, err := passwordUtils.HashPassword(password)
	suite.NoError(err)

	tests := []struct {
		name           string
		password       string
		getUserByLogin method[models.User]
		wantErr        error
	}{
		{
			name:     "Ok",
			password: password,
			getUserByLogin: method[models.User]{
				data: models.User{
					Id:           1,
					PasswordHash: passwordHash,
					Role:         models.RoleModerator,
				},
				err: nil,
			},
		},
		{
			name:     "OkEmptyRoleDefaultsToUser",
			password: password,
			getUserByLogin: method[models.User]{
				data: models.User{
					Id:           1,
					PasswordHash: passwordHash,
				},
				err: nil,
			},
		},
		{
			name:     "ErrUnauthorizedUnknownLogin",
			password: password,
			getUserByLogin: method[models.User]{
				err: repository.ErrNotFound,
			},
			wantErr: usecase.ErrUnauthorized,
		},
		{
			name:     "ErrUnauthorizedWrongPassword",
			password: "wrong",
			getUserByLogin: method[models.User]{
				data: models.User{
					Id:           1,
					PasswordHash: passwordHash,
				},
			},
			wantErr: usecase.ErrUnauthorized,
		},
		{
			name:     "ErrRepo",
			password: password,
			getUserByLogin: method[models.User]{
				err: errRepo,
			},
			wantErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.usersRepo.On("GetUserByLogin", mock.Anything, mock.AnythingOfType("string")).Once().
				Return(tt.getUserByLogin.data, tt.getUserByLogin.err)
			if tt.wantErr == nil {
				suite.expectIssue(1, 3, nil, nil)
			}

			accessToken, refreshToken, gotErr := suite.uc.SignIn(context.Background(), "login", tt.password)

			if tt.wantErr == nil {
				suite.NoError(gotErr)

				rc := suite.parseClaims(refreshToken, suite.authCfg.JWT.Refresh.Key)
				suite.Equal(token.TypeRefresh, rc.Type)
				suite.NotEmpty(rc.ID)
				suite.Equal(int64(3), rc.Version)
				ac := suite.parseClaims(accessToken, suite.authCfg.JWT.Access.Key)
				suite.Equal(token.TypeAccess, ac.Type)
				suite.Empty(ac.ID)
				suite.Equal(int64(3), ac.Version)

				wantRole := tt.getUserByLogin.data.Role
				if wantRole == "" {
					wantRole = models.RoleUser
				}
				parsed, err := jwt.Parse(accessToken, func(t *jwt.Token) (interface{}, error) {
					return []byte(suite.authCfg.JWT.Access.Key), nil
				})
				suite.NoError(err)
				claims, ok := parsed.Claims.(jwt.MapClaims)
				suite.True(ok)
				suite.Equal(string(wantRole), claims[token.RoleClaim])
				suite.Equal("1", claims["sub"])
			} else {
				suite.ErrorIs(gotErr, tt.wantErr)
			}
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *AuthSuite) TestSignIn_StoresFailOpen() {
	passwordHash, err := passwordUtils.HashPassword("password")
	suite.NoError(err)

	suite.usersRepo.On("GetUserByLogin", mock.Anything, "login").Once().
		Return(models.User{Id: 1, PasswordHash: passwordHash}, nil)
	suite.expectIssue(1, 0, errStore, errStore)

	accessToken, _, err := suite.uc.SignIn(context.Background(), "login", "password")
	suite.Require().NoError(err)
	suite.Equal(int64(0), suite.parseClaims(accessToken, suite.authCfg.JWT.Access.Key).Version)
}

func (suite *AuthSuite) TestSignIn_WithoutStores() {
	passwordHash, err := passwordUtils.HashPassword("password")
	suite.NoError(err)
	uc := usecase.NewAuth(suite.log, suite.authCfg, usecase.AuthRepositories{Users: suite.usersRepo})

	suite.usersRepo.On("GetUserByLogin", mock.Anything, "login").Once().
		Return(models.User{Id: 1, PasswordHash: passwordHash}, nil)

	_, refreshToken, err := uc.SignIn(context.Background(), "login", "password")
	suite.Require().NoError(err)
	suite.NotEmpty(suite.parseClaims(refreshToken, suite.authCfg.JWT.Refresh.Key).ID)
}

func (suite *AuthSuite) TestRefreshTokens() {
	const userId = 1
	const jti = "jti-1"

	accessAsRefresh, err := token.CreateToken(time.Hour, userId, string(models.RoleUser), suite.authCfg.JWT.Refresh.Key)
	suite.NoError(err)
	noJTI, err := token.Create(token.Params{TTL: time.Hour, UserID: userId, Type: token.TypeRefresh}, suite.authCfg.JWT.Refresh.Key)
	suite.NoError(err)
	wrongKey, err := token.Create(token.Params{TTL: time.Hour, UserID: userId, Type: token.TypeRefresh, ID: jti}, suite.authCfg.JWT.Access.Key)
	suite.NoError(err)

	tests := []struct {
		name          string
		token         string
		delete        method[bool]
		wantDelete    bool
		wantDeleteAll bool
		// version is the stored auth version checked after the id lookup;
		// nil when the check is not reached.
		version     *method[int64]
		getUserById method[models.User]
		wantIssue   bool
		wantErr     error
	}{
		{
			name:        "Ok",
			token:       suite.refreshToken(userId, jti),
			delete:      method[bool]{data: true},
			wantDelete:  true,
			version:     &method[int64]{data: currentVersion},
			getUserById: method[models.User]{data: models.User{Id: userId, Role: models.RoleModerator}},
			wantIssue:   true,
		},
		{
			name:        "OkStoreUnavailableFailsOpen",
			token:       suite.refreshToken(userId, jti),
			delete:      method[bool]{err: errStore},
			wantDelete:  true,
			version:     &method[int64]{data: currentVersion},
			getUserById: method[models.User]{data: models.User{Id: userId}},
			wantIssue:   true,
		},
		{
			name:        "OkVersionStoreUnavailableFailsOpen",
			token:       suite.refreshTokenV(userId, jti, 0),
			delete:      method[bool]{data: true},
			wantDelete:  true,
			version:     &method[int64]{err: errStore},
			getUserById: method[models.User]{data: models.User{Id: userId}},
			wantIssue:   true,
		},
		{
			name:       "ErrUnauthorizedStaleVersion",
			token:      suite.refreshTokenV(userId, jti, currentVersion-1),
			delete:     method[bool]{data: true},
			wantDelete: true,
			version:    &method[int64]{data: currentVersion},
			wantErr:    usecase.ErrUnauthorized,
		},
		{
			name:          "ErrUnauthorizedReuseRevokesAll",
			token:         suite.refreshToken(userId, jti),
			delete:        method[bool]{data: false},
			wantDelete:    true,
			wantDeleteAll: true,
			wantErr:       usecase.ErrUnauthorized,
		},
		{
			name:    "ErrUnauthorizedAccessTokenType",
			token:   accessAsRefresh,
			wantErr: usecase.ErrUnauthorized,
		},
		{
			name:    "ErrUnauthorizedNoJTI",
			token:   noJTI,
			wantErr: usecase.ErrUnauthorized,
		},
		{
			name:    "ErrUnauthorizedWrongKey",
			token:   wrongKey,
			wantErr: usecase.ErrUnauthorized,
		},
		{
			name:    "ErrUnauthorizedGarbage",
			token:   "not-a-jwt",
			wantErr: usecase.ErrUnauthorized,
		},
		{
			name:        "ErrUnauthorizedUserNotFound",
			token:       suite.refreshToken(userId, jti),
			delete:      method[bool]{data: true},
			wantDelete:  true,
			version:     &method[int64]{data: currentVersion},
			getUserById: method[models.User]{err: repository.ErrNotFound},
			wantErr:     usecase.ErrUnauthorized,
		},
		{
			name:        "ErrRepo",
			token:       suite.refreshToken(userId, jti),
			delete:      method[bool]{data: true},
			wantDelete:  true,
			version:     &method[int64]{data: currentVersion},
			getUserById: method[models.User]{err: errRepo},
			wantErr:     errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantDelete {
				suite.refresh.On("DeleteRefresh", mock.Anything, userId, jti).Once().Return(tt.delete.data, tt.delete.err)
			}
			if tt.wantDeleteAll {
				suite.refresh.On("DeleteAllRefresh", mock.Anything, userId).Once().Return(nil)
			}
			if tt.version != nil {
				suite.versions.On("AuthVersion", mock.Anything, userId).Once().Return(tt.version.data, tt.version.err)
			}
			if tt.getUserById.data.Id != 0 || tt.getUserById.err != nil {
				suite.usersRepo.On("GetUserById", mock.Anything, userId).Once().
					Return(tt.getUserById.data, tt.getUserById.err)
			}
			if tt.wantIssue {
				suite.expectIssue(userId, currentVersion, nil, nil)
			}

			accessToken, refreshToken, gotErr := suite.uc.RefreshTokens(context.Background(), tt.token)

			if tt.wantErr == nil {
				suite.Require().NoError(gotErr)
				rc := suite.parseClaims(refreshToken, suite.authCfg.JWT.Refresh.Key)
				suite.NotEqual(jti, rc.ID, "a new refresh id must be issued")
				suite.Equal(currentVersion, rc.Version)
				ac := suite.parseClaims(accessToken, suite.authCfg.JWT.Access.Key)
				suite.Equal(currentVersion, ac.Version)
				wantRole := tt.getUserById.data.Role
				if wantRole == "" {
					wantRole = models.RoleUser
				}
				suite.Equal(string(wantRole), ac.Role)
			} else {
				suite.ErrorIs(gotErr, tt.wantErr)
			}
		})
	}
}

func (suite *AuthSuite) TestRefreshTokens_WithoutStores() {
	uc := usecase.NewAuth(suite.log, suite.authCfg, usecase.AuthRepositories{Users: suite.usersRepo})
	suite.usersRepo.On("GetUserById", mock.Anything, 1).Once().Return(models.User{Id: 1}, nil)

	_, _, err := uc.RefreshTokens(context.Background(), suite.refreshToken(1, "jti"))
	suite.NoError(err)
}

func (suite *AuthSuite) TestLogout() {
	tests := []struct {
		name       string
		userID     int
		token      string
		wantDelete bool
		deleteErr  error
		wantErr    error
	}{
		{name: "Ok", userID: 1, token: suite.refreshToken(1, "jti"), wantDelete: true},
		{name: "OkAlreadyRevoked", userID: 1, token: suite.refreshToken(1, "jti"), wantDelete: true},
		{name: "OkStoreUnavailable", userID: 1, token: suite.refreshToken(1, "jti"), wantDelete: true, deleteErr: errStore},
		{name: "ErrUnauthorizedForeignToken", userID: 2, token: suite.refreshToken(1, "jti"), wantErr: usecase.ErrUnauthorized},
		{name: "ErrUnauthorizedGarbage", userID: 1, token: "abc", wantErr: usecase.ErrUnauthorized},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantDelete {
				suite.refresh.On("DeleteRefresh", mock.Anything, 1, "jti").Once().Return(tt.name == "Ok", tt.deleteErr)
			}

			err := suite.uc.Logout(context.Background(), tt.userID, tt.token)

			if tt.wantErr == nil {
				suite.NoError(err)
			} else {
				suite.ErrorIs(err, tt.wantErr)
			}
		})
	}
}

func (suite *AuthSuite) TestLogoutAll() {
	tests := []struct {
		name         string
		deleteAllErr error
		incrErr      error
	}{
		{name: "Ok"},
		{name: "OkRefreshStoreUnavailable", deleteAllErr: errStore},
		{name: "OkVersionStoreUnavailable", incrErr: errStore},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.refresh.On("DeleteAllRefresh", mock.Anything, 1).Once().Return(tt.deleteAllErr)
			suite.versions.On("IncrAuthVersion", mock.Anything, 1).Once().Return(int64(2), tt.incrErr)

			suite.NoError(suite.uc.LogoutAll(context.Background(), 1))
		})
	}
}
