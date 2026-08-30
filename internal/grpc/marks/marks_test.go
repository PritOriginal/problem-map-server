package marksgrpc_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/PritOriginal/problem-map-server/internal/grpc/interceptors"
	marksgrpc "github.com/PritOriginal/problem-map-server/internal/grpc/marks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MarksSuite struct {
	suite.Suite
	uc  *marksgrpc.MockMarks
	srv pb.MarksServer
}

func (suite *MarksSuite) SetupTest() {
	suite.uc = marksgrpc.NewMockMarks(suite.T())
	suite.srv = marksgrpc.New(slogdiscard.NewDiscardLogger(), suite.uc)
}

func TestMarks(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func authedCtx(userId int) context.Context {
	return interceptors.ContextWithClaims(context.Background(),
		interceptors.Claims{UserID: userId, Role: models.RoleUser})
}

func (suite *MarksSuite) TestGetMarks() {
	tests := []struct {
		name     string
		marks    []models.Mark
		err      error
		wantCode codes.Code
	}{
		{
			name: "Ok",
			marks: []models.Mark{
				{ID: 1, Description: "a", Geom: models.NewPoint(geom.Coord{1, 2}), MarkTypeID: 1, UserID: 3},
			},
			wantCode: codes.OK,
		},
		{name: "Internal", err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetMarks", mock.Anything, models.GetMarksFilters{}).Once().Return(tt.marks, tt.err)

			resp, err := suite.srv.GetMarks(context.Background(), &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Len(resp.GetMarks(), len(tt.marks))
				suite.Equal(int64(1), resp.GetMarks()[0].GetId())
				suite.Equal(2.0, resp.GetMarks()[0].GetGeom().GetCoordinates().GetLatitude())
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkById() {
	tests := []struct {
		name     string
		id       int64
		err      error
		wantCode codes.Code
	}{
		{name: "Ok", id: 1, wantCode: codes.OK},
		{name: "InvalidId", id: 0, wantCode: codes.InvalidArgument},
		{name: "NotFound", id: 2, err: usecase.ErrNotFound, wantCode: codes.NotFound},
		{name: "Internal", id: 3, err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCode != codes.InvalidArgument {
				suite.uc.On("GetMarkById", mock.Anything, int(tt.id)).Once().
					Return(models.Mark{ID: int(tt.id), Geom: models.NewPoint(geom.Coord{1, 2})}, tt.err)
			}

			resp, err := suite.srv.GetMarkById(context.Background(), &pb.GetMarkByIdRequest{MarkId: tt.id})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Equal(tt.id, resp.GetMark().GetId())
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarksByUserId() {
	tests := []struct {
		name     string
		userId   int64
		err      error
		wantCode codes.Code
	}{
		{name: "Ok", userId: 1, wantCode: codes.OK},
		{name: "InvalidId", userId: -1, wantCode: codes.InvalidArgument},
		{name: "Internal", userId: 3, err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCode != codes.InvalidArgument {
				suite.uc.On("GetMarksByUserId", mock.Anything, int(tt.userId)).Once().
					Return([]models.Mark{}, tt.err)
			}

			resp, err := suite.srv.GetMarksByUserId(context.Background(), &pb.GetMarksByUserIdRequest{UserId: tt.userId})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Empty(resp.GetMarks())
			}
		})
	}
}

func (suite *MarksSuite) TestAddMark() {
	validReq := func() *pb.AddMarkRequest {
		return &pb.AddMarkRequest{
			Point: &pb.Point{
				Type:        "Point",
				Coordinates: &pb.Coordinates{Longitude: 42, Latitude: 52},
			},
			MarkTypeId:  1,
			Description: "desc",
		}
	}

	tests := []struct {
		name     string
		ctx      context.Context
		mutate   func(*pb.AddMarkRequest)
		callUC   bool
		force    bool
		errAdd   error
		wantCode codes.Code
		// wantSimilar lists the mark ids expected as pb.Mark status details.
		wantSimilar []int64
	}{
		{name: "Ok", ctx: authedCtx(7), callUC: true, wantCode: codes.OK},
		{name: "Unauthenticated", ctx: context.Background(), wantCode: codes.Unauthenticated},
		{
			name: "NoPoint", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.Point = nil },
		},
		{
			name: "BadLongitude", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.Point.Coordinates.Longitude = 181 },
		},
		{
			name: "BadLatitude", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.Point.Coordinates.Latitude = -91 },
		},
		{
			name: "NaNLongitude", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.Point.Coordinates.Longitude = math.NaN() },
		},
		{
			name: "InfLatitude", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.Point.Coordinates.Latitude = math.Inf(1) },
		},
		{
			name: "NoMarkType", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.MarkTypeId = 0 },
		},
		{
			name: "LongDescription", ctx: authedCtx(7), wantCode: codes.InvalidArgument,
			mutate: func(r *pb.AddMarkRequest) { r.Description = strings.Repeat("A", 257) },
		},
		{name: "Internal", ctx: authedCtx(7), callUC: true, errAdd: errors.New("boom"), wantCode: codes.Internal},
		{
			name: "SimilarMarks", ctx: authedCtx(7), callUC: true, wantCode: codes.AlreadyExists,
			errAdd: fmt.Errorf("op: %w", &usecase.SimilarMarksError{Marks: []models.MarkWithDistance{
				{Mark: models.Mark{ID: 5, Geom: models.NewPoint(geom.Coord{42, 52})}, DistanceM: 12.5},
			}}),
			wantSimilar: []int64{5},
		},
		{
			name: "ForceMetadata", callUC: true, force: true, wantCode: codes.OK,
			ctx: metadata.NewIncomingContext(authedCtx(7), metadata.Pairs("force", "true")),
		},
		{
			name: "ForceMetadataInvalid", callUC: true, wantCode: codes.OK,
			ctx: metadata.NewIncomingContext(authedCtx(7), metadata.Pairs("force", "yes please")),
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			req := validReq()
			if tt.mutate != nil {
				tt.mutate(req)
			}
			if tt.callUC {
				suite.uc.On("AddMark", mock.Anything, mock.MatchedBy(func(m models.Mark) bool {
					c := m.Geom.Ewkb.Coords()
					return m.UserID == 7 && m.MarkTypeID == 1 && m.Description == "desc" &&
						c.X() == 42 && c.Y() == 52
				}), mock.Anything, tt.force).Once().Return(int64(10), tt.errAdd)
			}

			resp, err := suite.srv.AddMark(tt.ctx, req)
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Equal(int64(10), resp.GetMarkId())
			}
			if tt.wantSimilar != nil {
				var gotIds []int64
				for _, d := range status.Convert(err).Details() {
					m, ok := d.(*pb.Mark)
					suite.Require().True(ok, "detail %T", d)
					gotIds = append(gotIds, m.GetId())
				}
				suite.Equal(tt.wantSimilar, gotIds)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name     string
		md       metadata.MD
		wantLang models.Lang
		err      error
		wantCode codes.Code
	}{
		{name: "OkDefaultLang", wantLang: models.LangRU, wantCode: codes.OK},
		{name: "OkEN", md: metadata.Pairs("accept-language", "en-US,en;q=0.9"), wantLang: models.LangEN, wantCode: codes.OK},
		{name: "OkUnsupportedLang", md: metadata.Pairs("accept-language", "de"), wantLang: models.LangRU, wantCode: codes.OK},
		{name: "Internal", wantLang: models.LangRU, err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetMarkTypes", mock.Anything, tt.wantLang).Once().
				Return([]models.MarkType{{ID: 1, Code: "garbage", Name: "t"}}, tt.err)

			ctx := metadata.NewIncomingContext(context.Background(), tt.md)
			resp, err := suite.srv.GetMarkTypes(ctx, &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Len(resp.GetTypes(), 1)
			}
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{name: "Ok", wantCode: codes.OK},
		{name: "Internal", err: errors.New("boom"), wantCode: codes.Internal},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetMarkStatuses", mock.Anything, models.LangRU).Once().
				Return([]models.MarkStatus{{ID: 1, Code: "unconfirmed", Name: "s"}}, tt.err)

			resp, err := suite.srv.GetMarkStatuses(context.Background(), &emptypb.Empty{})
			suite.Equal(tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				suite.Len(resp.GetStatuses(), 1)
			}
		})
	}
}
