package tasksgrpc

import (
	"context"
	"errors"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Tasks interface {
	GetTasks(ctx context.Context) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}
type server struct {
	tasks Tasks
	pb.UnimplementedTasksServer
}

func Register(gRPCServer *grpc.Server, tasks Tasks) {
	pb.RegisterTasksServer(gRPCServer, &server{tasks: tasks})
}

func (s *server) GetTasks(ctx context.Context, in *emptypb.Empty) (*pb.GetTasksResponse, error) {
	tasks, err := s.tasks.GetTasks(context.Background())
	if err != nil {
		return nil, status.Error(codes.Internal, "error get tasks")
	}

	tasksPb := make([]*pb.Task, len(tasks))
	for i, task := range tasks {
		tasksPb[i] = task.ToProtobufObject()
	}

	return &pb.GetTasksResponse{
		Tasks: tasksPb,
	}, nil
}

func (s *server) GetTaskById(ctx context.Context, in *pb.GetTaskByIdRequest) (*pb.GetTaskByIdResponse, error) {
	// TODO: добавить валидацию.
	task, err := s.tasks.GetTaskById(ctx, int(in.GetId()))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "task not found")
		} else {
			return nil, status.Error(codes.Internal, "error get task by id")
		}
	}

	return &pb.GetTaskByIdResponse{
		Task: task.ToProtobufObject(),
	}, nil
}

func (s *server) GetTasksByUserId(ctx context.Context, in *pb.GetTasksByUserIdRequest) (*pb.GetTasksByUserIdResponse, error) {
	// TODO: добавить валидацию.
	tasks, err := s.tasks.GetTasksByUserId(ctx, int(in.GetUserId()))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "task not found")
		} else {
			return nil, status.Error(codes.Internal, "error get task by id")
		}
	}

	tasksPb := make([]*pb.Task, len(tasks))
	for i, task := range tasks {
		tasksPb[i] = task.ToProtobufObject()
	}

	return &pb.GetTasksByUserIdResponse{
		Tasks: tasksPb,
	}, nil
}

func (s *server) AddTask(ctx context.Context, in *pb.AddTaskRequest) (*pb.AddTaskResponse, error) {
	task := models.Task{
		Name:   in.GetName(),
		UserID: int(in.GetUserId()),
		MarkID: int(in.GetMarkId()),
	}

	taskId, err := s.tasks.AddTask(ctx, task)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed add task")
	}

	return &pb.AddTaskResponse{
		TaskId: taskId,
	}, nil
}
