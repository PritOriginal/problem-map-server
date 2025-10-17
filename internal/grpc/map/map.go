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
	GetMarks(ctx context.Context) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark) error
	PhotosRepository
}

type PhotosRepository interface {
	AddPhotos(photos [][]byte) error
	GetPhotos() error
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
