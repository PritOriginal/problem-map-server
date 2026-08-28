package mapgrpc

import (
	"context"
	"log/slog"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/grpcerr"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Map interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type server struct {
	log *slog.Logger
	uc  Map
	pb.UnimplementedMapServer
}

// New creates the Map gRPC service implementation.
func New(log *slog.Logger, uc Map) pb.MapServer {
	return &server{log: log, uc: uc}
}

func Register(gRPCServer *grpc.Server, log *slog.Logger, uc Map) {
	pb.RegisterMapServer(gRPCServer, New(log, uc))
}

func (s *server) GetRegions(ctx context.Context, in *emptypb.Empty) (*pb.GetRegionsResponse, error) {
	regions, err := s.uc.GetRegions(ctx)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get regions")
	}

	regionsPb := make([]*pb.Region, len(regions))
	for i := range regions {
		regionsPb[i] = regions[i].ToProtobufObject()
	}

	return &pb.GetRegionsResponse{
		Regions: regionsPb,
	}, nil
}

func (s *server) GetCities(ctx context.Context, in *emptypb.Empty) (*pb.GetCitiesResponse, error) {
	cities, err := s.uc.GetCities(ctx)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get cities")
	}

	citiesPb := make([]*pb.City, len(cities))
	for i := range cities {
		citiesPb[i] = cities[i].ToProtobufObject()
	}

	return &pb.GetCitiesResponse{
		Cities: citiesPb,
	}, nil
}

func (s *server) GetDistricts(ctx context.Context, in *emptypb.Empty) (*pb.GetDistrictsResponse, error) {
	districts, err := s.uc.GetDistricts(ctx)
	if err != nil {
		return nil, grpcerr.Map(s.log, err, "error get districts")
	}

	districtsPb := make([]*pb.District, len(districts))
	for i := range districts {
		districtsPb[i] = districts[i].ToProtobufObject()
	}

	return &pb.GetDistrictsResponse{
		Districts: districtsPb,
	}, nil
}
