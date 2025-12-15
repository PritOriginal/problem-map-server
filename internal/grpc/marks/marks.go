package marksgrpc

import (
	"context"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Marks interface {
	GetMarks(ctx context.Context) ([]models.Mark, error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark, photos [][]byte) (int64, error)
	GetMarkTypes(ctx context.Context) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error)
}

type server struct {
	uc Marks
	pb.UnimplementedMarksServer
}

func Register(gRPCServer *grpc.Server, uc Marks) {
	pb.RegisterMarksServer(gRPCServer, &server{uc: uc})
}

func (s *server) GetMarks(ctx context.Context, in *emptypb.Empty) (*pb.GetMarksResponse, error) {
	marks, err := s.uc.GetMarks(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "error get marks")
	}

	marksPb := make([]*pb.Mark, len(marks))
	for i, mark := range marks {
		marksPb[i] = mark.MarshalProtobuf()
	}

	return &pb.GetMarksResponse{
		Marks: marksPb,
	}, nil
}

func (s *server) AddMark(ctx context.Context, in *pb.AddMarkRequest) (*pb.AddMarkResponse, error) {
	return &pb.AddMarkResponse{}, nil
}
