package usersgrpc

import (
	"context"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type server struct {
	users usecase.Users
	pb.UnimplementedUsersServer
}

func Register(gRPCServer *grpc.Server, users usecase.Users) {
	pb.RegisterUsersServer(gRPCServer, &server{users: users})
}

func (s *server) AddUser(ctx context.Context, in *pb.AddUserRequest) (*pb.AddUserResponse, error) {
	user := models.User{
		Id:     int(in.GetUser().GetId()),
		Name:   in.GetUser().GetName(),
		Rating: int(in.GetUser().GetRating()),
	}

	id, err := s.users.AddUser(ctx, user)
	if err != nil {
		return nil, status.Error(codes.Internal, "error add user")
	}

	return &pb.AddUserResponse{
		UserId: id,
	}, nil
}

func (s *server) GetUserById(ctx context.Context, in *pb.GetUserByIdRequest) (*pb.GetUserByIdResponse, error) {
	id := in.GetId()

	user, err := s.users.GetUserById(ctx, int(id))
	if err != nil {
		return nil, status.Error(codes.Internal, "error get user by id")
	}

	return &pb.GetUserByIdResponse{
		User: user.MarshalProtobuf(),
	}, nil
}

func (s *server) GetUsers(ctx context.Context, in *emptypb.Empty) (*pb.GetUsersResponse, error) {
	users, err := s.users.GetUsers(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "error get users")
	}

	usersPb := make([]*pb.User, len(users))
	for i, user := range users {
		usersPb[i] = user.MarshalProtobuf()
	}

	return &pb.GetUsersResponse{
		Users: usersPb,
	}, nil
}
