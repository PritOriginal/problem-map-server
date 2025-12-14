package mapgrpc

import (
	"context"
	"fmt"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Map interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type server struct {
	uc Map
	pb.UnimplementedMapServer
}

func Register(gRPCServer *grpc.Server, uc Map) {
	pb.RegisterMapServer(gRPCServer, &server{uc: uc})
}

func (s *server) GetRegions(ctx context.Context, in *emptypb.Empty) (*pb.GetRegionsResponse, error) {
	regions, err := s.uc.GetRegions(ctx)
	if err != nil {
		fmt.Println(err.Error())
		return nil, status.Error(codes.Internal, "error get regions")
	}

	regionsPb := make([]*pb.Region, len(regions))
	for i, region := range regions {
		regionsPb[i] = region.MarshalProtobuf()
	}

	return &pb.GetRegionsResponse{
		Regions: regionsPb,
	}, nil
}

func (s *server) GetCities(ctx context.Context, in *emptypb.Empty) (*pb.GetCitiesResponse, error) {
	cities, err := s.uc.GetCities(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "error get cities")
	}

	citiesPb := make([]*pb.City, len(cities))
	for i, city := range cities {
		citiesPb[i] = city.MarshalProtobuf()
	}

	return &pb.GetCitiesResponse{
		Cities: citiesPb,
	}, nil
}

func (s *server) GetDistricts(ctx context.Context, in *emptypb.Empty) (*pb.GetDistrictsResponse, error) {
	districts, err := s.uc.GetDistricts(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "error get districts")
	}

	districtsPb := make([]*pb.District, len(districts))
	for i, district := range districts {
		districtsPb[i] = district.MarshalProtobuf()
	}

	return &pb.GetDistrictsResponse{
		Districts: districtsPb,
	}, nil
}
