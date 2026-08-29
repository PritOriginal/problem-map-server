package usersgrpc_test

import (
	"context"
	"errors"
	"testing"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	usersgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/users"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type UsersSuite struct {
	suite.Suite
	uc  *usersgrpc.MockUsers
	srv pb.UsersServer
}

func (suite *UsersSuite) SetupTest() {
	suite.uc = usersgrpc.NewMockUsers(suite.T())
	suite.srv = usersgrpc.New(slogdiscard.NewDiscardLogger(), suite.uc)
}

func TestUsers(t *testing.T) {
	suite.Run(t, new(UsersSuite))
}

func testUser(id int) models.User {
	return models.User{
		Id:        id,
		Name:      "name",
		Login:     "login",
		HomePoint: models.NewPoint(geom.Coord{1, 2}),
		Rating:    3,
		Role:      models.RoleUser,
	}
}

func authedCtx(userId int) context.Context {
	return interceptors.ContextWithClaims(context.Background(),
		interceptors.Claims{UserID: userId, Role: models.RoleUser})
}

func (suite *UsersSuite) TestGetUserById() {
	tests := []struct {
		name        string
		ctx         context.Context
		id          int64
		err         error
		wantCode    codes.Code
		wantPrivate bool
	}{
		{name: "OkAnonymousIsPublic", ctx: context.Background(), id: 1, wantCode: codes.OK},
		{name: "OkOtherUserIsPublic", ctx: authedCtx(2), id: 1, wantCode: codes.OK},
		{name: "OkOwnerIsFull", ctx: authedCtx(1), id: 1, wantCode: codes.OK, wantPrivate: true},
		{name: "InvalidId", ctx: context.Background(), id: 0, wantCode: codes.InvalidArgument},
		{name: "NotFound", ctx: context.Background(), id: 2, err: usecase.ErrNotFound, wantCode: codes.NotFound},
		{name: "Internal", ctx: context.Background(), id: 3, err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCode != codes.InvalidArgument {
				suite.uc.On("GetUserById", mock.Anything, int(tt.id)).Once().Return(testUser(int(tt.id)), tt.err)
			}

			resp, err := suite.srv.GetUserById(tt.ctx, &pb.GetUserByIdRequest{Id: tt.id})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode != codes.OK {
				return
			}

			user := resp.GetUser()
			suite.Equal(tt.id, user.GetId())
			suite.Equal("name", user.GetName())
			suite.Equal(int64(3), user.GetRating())
			if tt.wantPrivate {
				suite.Equal("login", user.GetLogin())
				suite.NotNil(user.GetHomePoint())
			} else {
				suite.Empty(user.GetLogin())
				suite.Nil(user.GetHomePoint())
			}
		})
	}
}

func (suite *UsersSuite) TestGetUsers() {
	tests := []struct {
		name     string
		ctx      context.Context
		err      error
		wantCode codes.Code
	}{
		{name: "OkAnonymous", ctx: context.Background(), wantCode: codes.OK},
		{name: "OkAuthed", ctx: authedCtx(1), wantCode: codes.OK},
		{name: "Internal", ctx: context.Background(), err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetUsers", mock.Anything).Once().
				Return([]models.User{testUser(1), testUser(2)}, tt.err)

			resp, err := suite.srv.GetUsers(tt.ctx, &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode != codes.OK {
				return
			}

			suite.Require().Len(resp.GetUsers(), 2)
			for _, user := range resp.GetUsers() {
				_, authed := interceptors.ClaimsFromContext(tt.ctx)
				if authed && user.GetId() == 1 {
					suite.Equal("login", user.GetLogin())
					suite.NotNil(user.GetHomePoint())
				} else {
					suite.Empty(user.GetLogin())
					suite.Nil(user.GetHomePoint())
				}
			}
		})
	}
}
