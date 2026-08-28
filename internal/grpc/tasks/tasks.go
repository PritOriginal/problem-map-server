package tasksgrpc

import (
	"context"
	"log/slog"
	"strings"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/grpcerr"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Tasks interface {
	GetTasks(ctx context.Context, filters models.GetTasksFilters) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}

type server struct {
	log   *slog.Logger
	tasks Tasks
	pb.UnimplementedTasksServer
}

// New creates the Tasks gRPC service implementation.
func New(log *slog.Logger, tasks Tasks) pb.TasksServer {
	return &server{log: log, tasks: tasks}
}

func Register(gRPCServer *grpc.Server, log *slog.Logger, tasks Tasks) {
	pb.RegisterTasksServer(gRPCServer, New(log, tasks))
}

func (s *server) GetTasks(ctx context.Context, in *emptypb.Empty) (*pb.GetTasksResponse, error) {
	tasks, err := s.tasks.GetTasks(ctx, models.GetTasksFilters{})
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get tasks")
	}

	tasksPb := make([]*pb.Task, len(tasks))
	for i := range tasks {
		tasksPb[i] = tasks[i].ToProtobufObject()
	}

	return &pb.GetTasksResponse{
		Tasks: tasksPb,
	}, nil
}

func (s *server) GetTaskById(ctx context.Context, in *pb.GetTaskByIdRequest) (*pb.GetTaskByIdResponse, error) {
	id := in.GetId()
	if id <= 0 {
		return nil, grpcerr.InvalidArgument("id must be positive")
	}

	task, err := s.tasks.GetTaskById(ctx, int(id))
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get task by id", slog.Int64("task_id", id))
	}

	return &pb.GetTaskByIdResponse{
		Task: task.ToProtobufObject(),
	}, nil
}

func (s *server) GetTasksByUserId(ctx context.Context, in *pb.GetTasksByUserIdRequest) (*pb.GetTasksByUserIdResponse, error) {
	userId := in.GetUserId()
	if userId <= 0 {
		return nil, grpcerr.InvalidArgument("user_id must be positive")
	}

	tasks, err := s.tasks.GetTasksByUserId(ctx, int(userId), models.GetTasksByUserIdFilters{})
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get tasks by user id", slog.Int64("user_id", userId))
	}

	tasksPb := make([]*pb.Task, len(tasks))
	for i := range tasks {
		tasksPb[i] = tasks[i].ToProtobufObject()
	}

	return &pb.GetTasksByUserIdResponse{
		Tasks: tasksPb,
	}, nil
}

// AddTask creates a task on behalf of the authenticated moderator/admin.
// As in REST, the task owner is taken from the token; the request user_id
// is ignored. Role checks are enforced by the interceptors.
func (s *server) AddTask(ctx context.Context, in *pb.AddTaskRequest) (*pb.AddTaskResponse, error) {
	claims, ok := interceptors.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if strings.TrimSpace(in.GetName()) == "" {
		return nil, grpcerr.InvalidArgument("name is required")
	}
	if in.GetMarkId() <= 0 {
		return nil, grpcerr.InvalidArgument("mark_id must be positive")
	}

	task := models.Task{
		Name:   in.GetName(),
		UserID: claims.UserID,
		MarkID: int(in.GetMarkId()),
	}

	taskId, err := s.tasks.AddTask(ctx, task)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error add task",
			slog.Int("user_id", claims.UserID), slog.Int64("mark_id", in.GetMarkId()))
	}

	s.log.Info("add new task", slog.Int64("task_id", taskId), slog.Int("user_id", claims.UserID))

	return &pb.AddTaskResponse{
		TaskId: taskId,
	}, nil
}
