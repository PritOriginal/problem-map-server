package interceptors_test

import (
	"context"
	"testing"
	"time"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testKey = "1234"

type AuthSuite struct {
	suite.Suite
	auth *interceptors.Auth
}

func (suite *AuthSuite) SetupSuite() {
	suite.auth = interceptors.NewAuth(slogdiscard.NewDiscardLogger(), testKey)
}

func TestAuth(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func mustToken(t *testing.T, userId int, role string, key string) string {
	t.Helper()
	tok, err := token.CreateToken(time.Hour, userId, role, key)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func ctxWithAuthorization(value string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", value))
}

func (suite *AuthSuite) TestAuthFunc() {
	tests := []struct {
		name       string
		ctx        context.Context
		wantCode   codes.Code
		wantClaims *interceptors.Claims
	}{
		{
			name:     "NoHeaderIsAnonymous",
			ctx:      context.Background(),
			wantCode: codes.OK,
		},
		{
			name:     "ValidToken",
			ctx:      ctxWithAuthorization("Bearer " + mustToken(suite.T(), 42, "moderator", testKey)),
			wantCode: codes.OK,
			wantClaims: &interceptors.Claims{
				UserID: 42,
				Role:   models.RoleModerator,
			},
		},
		{
			name:     "LowercaseScheme",
			ctx:      ctxWithAuthorization("bearer " + mustToken(suite.T(), 7, "", testKey)),
			wantCode: codes.OK,
			wantClaims: &interceptors.Claims{
				UserID: 7,
				Role:   models.RoleUser,
			},
		},
		{
			name:     "WrongKey",
			ctx:      ctxWithAuthorization("Bearer " + mustToken(suite.T(), 42, "user", "other")),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "RefreshToken",
			ctx:      ctxWithAuthorization("Bearer " + mustRefreshToken(suite.T(), 42, testKey)),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "WrongScheme",
			ctx:      ctxWithAuthorization("Basic abc"),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "Garbage",
			ctx:      ctxWithAuthorization("Bearer not-a-jwt"),
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "EmptyToken",
			ctx:      ctxWithAuthorization("Bearer "),
			wantCode: codes.Unauthenticated,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ctx, err := suite.auth.AuthFunc(tt.ctx)
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode != codes.OK {
				return
			}

			claims, ok := interceptors.ClaimsFromContext(ctx)
			if tt.wantClaims == nil {
				suite.False(ok)
				return
			}
			suite.True(ok)
			suite.Equal(*tt.wantClaims, claims)
		})
	}
}

func okHandler(_ context.Context, _ any) (any, error) { return "ok", nil }

func (suite *AuthSuite) TestRequireAuth() {
	interceptor := interceptors.RequireAuth(pb.Marks_AddMark_FullMethodName)
	authed := interceptors.ContextWithClaims(context.Background(), interceptors.Claims{UserID: 1, Role: models.RoleUser})

	tests := []struct {
		name     string
		ctx      context.Context
		method   string
		wantCode codes.Code
	}{
		{name: "ProtectedAnonymous", ctx: context.Background(), method: pb.Marks_AddMark_FullMethodName, wantCode: codes.Unauthenticated},
		{name: "ProtectedAuthed", ctx: authed, method: pb.Marks_AddMark_FullMethodName, wantCode: codes.OK},
		{name: "PublicAnonymous", ctx: context.Background(), method: pb.Marks_GetMarks_FullMethodName, wantCode: codes.OK},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			_, err := interceptor(tt.ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method}, okHandler)
			suite.Equal(tt.wantCode, status.Code(err))
		})
	}
}

func (suite *AuthSuite) TestRequireRole() {
	interceptor := interceptors.RequireRole(
		[]models.Role{models.RoleModerator, models.RoleAdmin}, pb.Tasks_AddTask_FullMethodName)

	withRole := func(role models.Role) context.Context {
		return interceptors.ContextWithClaims(context.Background(), interceptors.Claims{UserID: 1, Role: role})
	}

	tests := []struct {
		name     string
		ctx      context.Context
		method   string
		wantCode codes.Code
	}{
		{name: "Anonymous", ctx: context.Background(), method: pb.Tasks_AddTask_FullMethodName, wantCode: codes.Unauthenticated},
		{name: "User", ctx: withRole(models.RoleUser), method: pb.Tasks_AddTask_FullMethodName, wantCode: codes.PermissionDenied},
		{name: "Moderator", ctx: withRole(models.RoleModerator), method: pb.Tasks_AddTask_FullMethodName, wantCode: codes.OK},
		{name: "Admin", ctx: withRole(models.RoleAdmin), method: pb.Tasks_AddTask_FullMethodName, wantCode: codes.OK},
		{name: "OtherMethodUser", ctx: withRole(models.RoleUser), method: pb.Tasks_GetTasks_FullMethodName, wantCode: codes.OK},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			_, err := interceptor(tt.ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method}, okHandler)
			suite.Equal(tt.wantCode, status.Code(err))
		})
	}
}

// mustRefreshToken issues a refresh-typed token signed with key: it must
// not be accepted as a bearer token even though the signature is valid.
func mustRefreshToken(t *testing.T, userID int, key string) string {
	t.Helper()
	tok, err := token.Create(token.Params{TTL: time.Minute, UserID: userID, Role: "admin", Type: token.TypeRefresh, ID: "jti"}, key)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
