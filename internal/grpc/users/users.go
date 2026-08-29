package usersgrpc

import (
	"context"
	"log/slog"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/grpcerr"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Users interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUsers(ctx context.Context) ([]models.User, error)
}

type server struct {
	log   *slog.Logger
	users Users
	pb.UnimplementedUsersServer
}

// New creates the Users gRPC service implementation.
func New(log *slog.Logger, users Users) pb.UsersServer {
	return &server{log: log, users: users}
}

func Register(gRPCServer *grpc.Server, log *slog.Logger, users Users) {
	pb.RegisterUsersServer(gRPCServer, New(log, users))
}

func (s *server) GetUserById(ctx context.Context, in *pb.GetUserByIdRequest) (*pb.GetUserByIdResponse, error) {
	id := in.GetId()
	if id <= 0 {
		return nil, grpcerr.InvalidArgument("id must be positive")
	}

	user, err := s.users.GetUserById(ctx, int(id))
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get user by id", slog.Int64("user_id", id))
	}

	return &pb.GetUserByIdResponse{
		User: toProtobufObject(ctx, user),
	}, nil
}

func (s *server) GetUsers(ctx context.Context, in *emptypb.Empty) (*pb.GetUsersResponse, error) {
	users, err := s.users.GetUsers(ctx)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get users")
	}

	usersPb := make([]*pb.User, len(users))
	for i := range users {
		usersPb[i] = toProtobufObject(ctx, users[i])
	}

	return &pb.GetUsersResponse{
		Users: usersPb,
	}, nil
}

// toProtobufObject mirrors the REST PublicUser: private fields (login and
// home point) are exposed only to the authenticated owner of the profile.
func toProtobufObject(ctx context.Context, user models.User) *pb.User {
	result := user.ToProtobufObject()

	claims, ok := interceptors.ClaimsFromContext(ctx)
	if !ok || claims.UserID != user.Id {
		result.Login = ""
		result.HomePoint = nil
	}

	return result
}
