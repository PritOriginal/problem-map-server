package mapgrpc_test

import (
	"context"
	"errors"
	"testing"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	mapgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/map"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MapSuite struct {
	suite.Suite
	uc  *mapgrpc.MockMap
	srv pb.MapServer
}

func (suite *MapSuite) SetupTest() {
	suite.uc = mapgrpc.NewMockMap(suite.T())
	suite.srv = mapgrpc.New(slogdiscard.NewDiscardLogger(), suite.uc)
}

func TestMap(t *testing.T) {
	suite.Run(t, new(MapSuite))
}

func testPolygon() *models.Polygon {
	return models.NewPolygon([][]geom.Coord{{{0, 0}, {1, 0}, {1, 1}, {0, 0}}})
}

type errCase struct {
	name     string
	err      error
	wantCode codes.Code
}

var errCases = []errCase{
	{name: "Ok", wantCode: codes.OK},
	{name: "Internal", err: errors.New("boom"), wantCode: codes.Internal},
}

func (suite *MapSuite) TestGetRegions() {
	for _, tt := range errCases {
		suite.Run(tt.name, func() {
			suite.uc.On("GetRegions", mock.Anything).Once().
				Return([]models.Region{{ID: 1, Name: "r", Geom: testPolygon()}, {ID: 2, Name: "nil-geom"}}, tt.err)

			resp, err := suite.srv.GetRegions(context.Background(), &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Require().Len(resp.GetRegions(), 2)
				suite.Len(resp.GetRegions()[0].GetGeom().GetCoordinates(), 4)
				suite.Nil(resp.GetRegions()[1].GetGeom())
			}
		})
	}
}

func (suite *MapSuite) TestGetCities() {
	for _, tt := range errCases {
		suite.Run(tt.name, func() {
			suite.uc.On("GetCities", mock.Anything).Once().
				Return([]models.City{{ID: 1, Name: "c", RegionID: 2, Geom: testPolygon()}}, tt.err)

			resp, err := suite.srv.GetCities(context.Background(), &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Require().Len(resp.GetCities(), 1)
				suite.Equal(int64(2), resp.GetCities()[0].GetRegionId())
				suite.Len(resp.GetCities()[0].GetGeom().GetCoordinates(), 4)
			}
		})
	}
}

func (suite *MapSuite) TestGetDistricts() {
	for _, tt := range errCases {
		suite.Run(tt.name, func() {
			suite.uc.On("GetDistricts", mock.Anything).Once().
				Return([]models.District{{ID: 1, Name: "d", CityID: 3, Geom: testPolygon()}}, tt.err)

			resp, err := suite.srv.GetDistricts(context.Background(), &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Require().Len(resp.GetDistricts(), 1)
				suite.Equal(int64(3), resp.GetDistricts()[0].GetCityId())
			}
		})
	}
}
