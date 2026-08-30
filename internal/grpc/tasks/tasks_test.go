package tasksgrpc_test

import (
	"context"
	"errors"
	"testing"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	tasksgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/tasks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TasksSuite struct {
	suite.Suite
	uc  *tasksgrpc.MockTasks
	srv pb.TasksServer
}

func (suite *TasksSuite) SetupTest() {
	suite.uc = tasksgrpc.NewMockTasks(suite.T())
	suite.srv = tasksgrpc.New(slogdiscard.NewDiscardLogger(), suite.uc)
}

func TestTasks(t *testing.T) {
	suite.Run(t, new(TasksSuite))
}

func (suite *TasksSuite) TestGetTasks() {
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{name: "Ok", wantCode: codes.OK},
		{name: "Internal", err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetTasks", mock.Anything, models.GetTasksFilters{}).Once().
				Return([]models.Task{{ID: 1, Name: "t", UserID: 2, MarkID: 3, StatusID: models.UnfulfilledStatus}}, tt.err)

			resp, err := suite.srv.GetTasks(context.Background(), &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Len(resp.GetTasks(), 1)
				suite.Equal(int64(1), resp.GetTasks()[0].GetId())
			}
		})
	}
}

func (suite *TasksSuite) TestGetTaskById() {
	tests := []struct {
		name     string
		id       int64
		err      error
		wantCode codes.Code
	}{
		{name: "Ok", id: 1, wantCode: codes.OK},
		{name: "InvalidId", id: 0, wantCode: codes.InvalidArgument},
		{name: "NotFound", id: 2, err: usecase.ErrNotFound, wantCode: codes.NotFound},
		{name: "Internal", id: 3, err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCode != codes.InvalidArgument {
				suite.uc.On("GetTaskById", mock.Anything, int(tt.id)).Once().
					Return(models.Task{ID: int(tt.id)}, tt.err)
			}

			resp, err := suite.srv.GetTaskById(context.Background(), &pb.GetTaskByIdRequest{Id: tt.id})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Equal(tt.id, resp.GetTask().GetId())
			}
		})
	}
}

func (suite *TasksSuite) TestGetTasksByUserId() {
	tests := []struct {
		name     string
		userId   int64
		err      error
		wantCode codes.Code
	}{
		{name: "Ok", userId: 1, wantCode: codes.OK},
		{name: "InvalidId", userId: 0, wantCode: codes.InvalidArgument},
		{name: "Internal", userId: 3, err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCode != codes.InvalidArgument {
				suite.uc.On("GetTasksByUserId", mock.Anything, int(tt.userId), models.GetTasksByUserIdFilters{}).Once().
					Return([]models.Task{}, tt.err)
			}

			resp, err := suite.srv.GetTasksByUserId(context.Background(), &pb.GetTasksByUserIdRequest{UserId: tt.userId})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Empty(resp.GetTasks())
			}
		})
	}
}

func (suite *TasksSuite) TestAddTask() {
	moderatorCtx := interceptors.ContextWithClaims(context.Background(),
		interceptors.Claims{UserID: 7, Role: models.RoleModerator})

	tests := []struct {
		name     string
		ctx      context.Context
		req      *pb.AddTaskRequest
		callUC   bool
		errAdd   error
		wantCode codes.Code
	}{
		{
			name: "Ok", ctx: moderatorCtx, callUC: true, wantCode: codes.OK,
			// user_id from the request is ignored in favour of the token.
			req: &pb.AddTaskRequest{Name: "task", UserId: 99, MarkId: 5},
		},
		{
			name: "Unauthenticated", ctx: context.Background(), wantCode: codes.Unauthenticated,
			req: &pb.AddTaskRequest{Name: "task", UserId: 99, MarkId: 5},
		},
		{
			name: "EmptyName", ctx: moderatorCtx, wantCode: codes.InvalidArgument,
			req: &pb.AddTaskRequest{Name: "  ", UserId: 99, MarkId: 5},
		},
		{
			name: "NoMarkId", ctx: moderatorCtx, wantCode: codes.InvalidArgument,
			req: &pb.AddTaskRequest{Name: "task", UserId: 99},
		},
		{
			name: "NoUserId", ctx: moderatorCtx, wantCode: codes.InvalidArgument,
			req: &pb.AddTaskRequest{Name: "task", MarkId: 5},
		},
		{
			name: "Internal", ctx: moderatorCtx, callUC: true, errAdd: errors.New("boom"), wantCode: codes.Internal,
			req: &pb.AddTaskRequest{Name: "task", UserId: 99, MarkId: 5},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callUC {
				// user_id is the assignee from the request, not the moderator (7) from the claims.
				suite.uc.On("AddTask", mock.Anything, models.Task{Name: "task", UserID: 99, MarkID: 5}).Once().
					Return(int64(11), tt.errAdd)
			}

			resp, err := suite.srv.AddTask(tt.ctx, tt.req)
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Equal(int64(11), resp.GetTaskId())
			}
		})
	}
}
