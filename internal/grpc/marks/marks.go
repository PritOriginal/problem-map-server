package marksgrpc

import (
	"context"
	"io"
	"log/slog"
	"math"
	"unicode/utf8"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/grpcerr"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	"github.com/PritOriginal/problem-map-server/internal/grpc/pbconv"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/twpayne/go-geom"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// maxDescriptionLen mirrors the REST AddMarkRequest binding (max=256).
const maxDescriptionLen = 256

type Marks interface {
	GetMarks(ctx context.Context, filters models.GetMarksFilters) ([]models.Mark, error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark, photos []io.Reader) (int64, error)
	GetMarkTypes(ctx context.Context) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error)
}

type server struct {
	log *slog.Logger
	uc  Marks
	pb.UnimplementedMarksServer
}

// New creates the Marks gRPC service implementation.
func New(log *slog.Logger, uc Marks) pb.MarksServer {
	return &server{log: log, uc: uc}
}

func Register(gRPCServer *grpc.Server, log *slog.Logger, uc Marks) {
	pb.RegisterMarksServer(gRPCServer, New(log, uc))
}

func (s *server) GetMarks(ctx context.Context, in *emptypb.Empty) (*pb.GetMarksResponse, error) {
	marks, err := s.uc.GetMarks(ctx, models.GetMarksFilters{})
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get marks")
	}

	marksPb := pbconv.Slice(marks, (*models.Mark).ToProtobufObject)

	return &pb.GetMarksResponse{
		Marks: marksPb,
	}, nil
}

func (s *server) GetMarkById(ctx context.Context, in *pb.GetMarkByIdRequest) (*pb.GetMarkByIdResponse, error) {
	id := in.GetMarkId()
	if id <= 0 {
		return nil, grpcerr.InvalidArgument("mark_id must be positive")
	}

	mark, err := s.uc.GetMarkById(ctx, int(id))
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get mark by id", slog.Int64("mark_id", id))
	}

	return &pb.GetMarkByIdResponse{
		Mark: mark.ToProtobufObject(),
	}, nil
}

func (s *server) GetMarksByUserId(ctx context.Context, in *pb.GetMarksByUserIdRequest) (*pb.GetMarksByUserIdResponse, error) {
	userId := in.GetUserId()
	if userId <= 0 {
		return nil, grpcerr.InvalidArgument("user_id must be positive")
	}

	marks, err := s.uc.GetMarksByUserId(ctx, int(userId))
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get marks by user id", slog.Int64("user_id", userId))
	}

	marksPb := pbconv.Slice(marks, (*models.Mark).ToProtobufObject)

	return &pb.GetMarksByUserIdResponse{
		Marks: marksPb,
	}, nil
}

// AddMark creates a mark for the authenticated user. The protobuf request
// carries no photo payload, so the mark is created without photos (unlike
// the REST endpoint, where photos are required).
func (s *server) AddMark(ctx context.Context, in *pb.AddMarkRequest) (*pb.AddMarkResponse, error) {
	claims, ok := interceptors.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := validateAddMark(in); err != nil {
		s.log.Debug("invalid add mark request", logger.Err(err))
		return nil, err
	}

	coords := in.GetPoint().GetCoordinates()
	newMark := models.Mark{
		Geom:        models.NewPoint(geom.Coord{coords.GetLongitude(), coords.GetLatitude()}),
		MarkTypeID:  int(in.GetMarkTypeId()),
		UserID:      claims.UserID,
		Description: in.GetDescription(),
	}

	markId, err := s.uc.AddMark(ctx, newMark, nil)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error add mark", slog.Int("user_id", claims.UserID))
	}

	s.log.Info("add new mark",
		slog.Int64("mark_id", markId),
		slog.Int("user_id", claims.UserID),
		slog.Float64("longitude", coords.GetLongitude()),
		slog.Float64("latitude", coords.GetLatitude()),
	)

	return &pb.AddMarkResponse{
		MarkId: markId,
	}, nil
}

// validateAddMark mirrors the REST AddMarkRequest binding rules and returns
// a codes.InvalidArgument status on failure.
func validateAddMark(in *pb.AddMarkRequest) error {
	coords := in.GetPoint().GetCoordinates()
	if coords == nil {
		return grpcerr.InvalidArgument("point is required")
	}
	// NaN passes plain range comparisons, so it is rejected explicitly.
	if lon := coords.GetLongitude(); math.IsNaN(lon) || lon < -180 || lon > 180 {
		return grpcerr.InvalidArgument("longitude must be in [-180, 180]")
	}
	if lat := coords.GetLatitude(); math.IsNaN(lat) || lat < -90 || lat > 90 {
		return grpcerr.InvalidArgument("latitude must be in [-90, 90]")
	}
	if in.GetMarkTypeId() <= 0 {
		return grpcerr.InvalidArgument("mark_type_id must be positive")
	}
	if utf8.RuneCountInString(in.GetDescription()) > maxDescriptionLen {
		return grpcerr.InvalidArgument("description is too long")
	}

	return nil
}

func (s *server) GetMarkTypes(ctx context.Context, in *emptypb.Empty) (*pb.GetMarkTypesResponse, error) {
	types, err := s.uc.GetMarkTypes(ctx)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get mark types")
	}

	typesPb := pbconv.Slice(types, (*models.MarkType).ToProtobufObject)

	return &pb.GetMarkTypesResponse{
		Types: typesPb,
	}, nil
}

func (s *server) GetMarkStatuses(ctx context.Context, in *emptypb.Empty) (*pb.GetMarkStatusesResponse, error) {
	statuses, err := s.uc.GetMarkStatuses(ctx)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get mark statuses")
	}

	statusesPb := pbconv.Slice(statuses, (*models.MarkStatus).ToProtobufObject)

	return &pb.GetMarkStatusesResponse{
		Statuses: statusesPb,
	}, nil
}
