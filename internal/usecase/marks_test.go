package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MarksSuite struct {
	suite.Suite
	uc         *usecase.Marks
	log        *slog.Logger
	trManager  *usecase.MockManager
	marksRepo  *usecase.MockMarksRepository
	checksRepo *usecase.MockChecksRepository
	photosRepo *usecase.MockPhotosRepository
}

func (suite *MarksSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.uc = usecase.NewMarks(suite.log, suite.trManager, usecase.MarksRepositories{
		Marks:  suite.marksRepo,
		Checks: suite.checksRepo,
		Photos: suite.photosRepo,
	})
}

func TestMarks(t *testing.T) {
	suite.Run(t, new(MarksSuite))
}

func (suite *MarksSuite) TestGetMarks() {
	tests := []struct {
		name     string
		getMarks method[models.Page[models.Mark]]
	}{
		{
			name: "Ok",
			getMarks: method[models.Page[models.Mark]]{
				data: models.Page[models.Mark]{Items: []models.Mark{{}, {}}, Total: 2},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarks: method[models.Page[models.Mark]]{
				data: models.Page[models.Mark]{},
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// GetMarks must always ask the repository for the full set.
			suite.marksRepo.On("GetMarks", mock.Anything, mock.MatchedBy(func(f models.GetMarksFilters) bool {
				return f.Pagination.IsZero()
			})).Once().Return(tt.getMarks.data, tt.getMarks.err)

			got, gotErr := suite.uc.GetMarks(context.Background(), models.GetMarksFilters{
				Pagination: models.Pagination{Limit: 10, Offset: 5},
			})

			if tt.getMarks.err == nil {
				suite.NoError(gotErr)
				suite.Equal(tt.getMarks.data.Items, got)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestListMarks() {
	tests := []struct {
		name       string
		filters    models.GetMarksFilters
		callRepo   bool
		getMarks   method[models.Page[models.Mark]]
		wantErrArg bool
	}{
		{
			name:     "Ok",
			filters:  models.GetMarksFilters{Pagination: models.Pagination{Limit: 10}},
			callRepo: true,
			getMarks: method[models.Page[models.Mark]]{
				data: models.Page[models.Mark]{Items: []models.Mark{{}}, Total: 1},
			},
		},
		{
			name: "OkAllFilters",
			filters: models.GetMarksFilters{
				BBox:        &models.BBox{MinLon: 41.4, MinLat: 52.7, MaxLon: 41.5, MaxLat: 52.8},
				Sort:        models.MarksSortUpdatedAt,
				Order:       models.SortAsc,
				CreatedFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				CreatedTo:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
				Pagination:  models.Pagination{Limit: 500, Offset: 10},
			},
			callRepo: true,
		},
		{
			name:     "ErrRepo",
			filters:  models.GetMarksFilters{Pagination: models.Pagination{Limit: 10}},
			callRepo: true,
			getMarks: method[models.Page[models.Mark]]{err: errors.New("")},
		},
		{
			name:       "ErrLimitTooBig",
			filters:    models.GetMarksFilters{Pagination: models.Pagination{Limit: models.MaxLimit + 1}},
			wantErrArg: true,
		},
		{
			name:       "ErrNegativeOffset",
			filters:    models.GetMarksFilters{Pagination: models.Pagination{Limit: 10, Offset: -1}},
			wantErrArg: true,
		},
		{
			name:       "ErrBBox",
			filters:    models.GetMarksFilters{BBox: &models.BBox{MinLon: 2, MinLat: 0, MaxLon: 1, MaxLat: 1}},
			wantErrArg: true,
		},
		{
			name:       "ErrSort",
			filters:    models.GetMarksFilters{Sort: "description"},
			wantErrArg: true,
		},
		{
			name:       "ErrOrder",
			filters:    models.GetMarksFilters{Order: "random"},
			wantErrArg: true,
		},
		{
			name: "ErrCreatedRange",
			filters: models.GetMarksFilters{
				CreatedFrom: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
				CreatedTo:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			wantErrArg: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callRepo {
				suite.marksRepo.On("GetMarks", mock.Anything, tt.filters).Once().
					Return(tt.getMarks.data, tt.getMarks.err)
			}

			got, gotErr := suite.uc.ListMarks(context.Background(), tt.filters)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.getMarks.err != nil:
				suite.Error(gotErr)
				suite.NotErrorIs(gotErr, usecase.ErrInvalidArgument)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.getMarks.data, got)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarksNearby() {
	okFilters := models.GetMarksNearbyFilters{Lon: 41.45, Lat: 52.72, RadiusM: 1000, Pagination: models.Pagination{Limit: 20}}

	tests := []struct {
		name       string
		filters    models.GetMarksNearbyFilters
		callRepo   bool
		nearby     method[models.Page[models.MarkWithDistance]]
		wantErrArg bool
	}{
		{
			name:     "Ok",
			filters:  okFilters,
			callRepo: true,
			nearby: method[models.Page[models.MarkWithDistance]]{
				data: models.Page[models.MarkWithDistance]{Items: []models.MarkWithDistance{{DistanceM: 12.5}}, Total: 1},
			},
		},
		{
			name:     "ErrRepo",
			filters:  okFilters,
			callRepo: true,
			nearby:   method[models.Page[models.MarkWithDistance]]{err: errors.New("")},
		},
		{
			name:       "ErrRadiusZero",
			filters:    models.GetMarksNearbyFilters{Lon: 41.45, Lat: 52.72, RadiusM: 0},
			wantErrArg: true,
		},
		{
			name:       "ErrRadiusTooBig",
			filters:    models.GetMarksNearbyFilters{Lon: 41.45, Lat: 52.72, RadiusM: usecase.MaxNearbyRadiusM + 1},
			wantErrArg: true,
		},
		{
			name:       "ErrLongitude",
			filters:    models.GetMarksNearbyFilters{Lon: 181, Lat: 52.72, RadiusM: 100},
			wantErrArg: true,
		},
		{
			name:       "ErrLatitude",
			filters:    models.GetMarksNearbyFilters{Lon: 41.45, Lat: -91, RadiusM: 100},
			wantErrArg: true,
		},
		{
			name:       "ErrPagination",
			filters:    models.GetMarksNearbyFilters{Lon: 41.45, Lat: 52.72, RadiusM: 100, Pagination: models.Pagination{Limit: -1}},
			wantErrArg: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callRepo {
				suite.marksRepo.On("GetMarksNearby", mock.Anything, tt.filters).Once().
					Return(tt.nearby.data, tt.nearby.err)
			}

			got, gotErr := suite.uc.GetMarksNearby(context.Background(), tt.filters)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.nearby.err != nil:
				suite.Error(gotErr)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.nearby.data, got)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestListMarksByUserId() {
	tests := []struct {
		name       string
		pagination models.Pagination
		callRepo   bool
		page       method[models.Page[models.Mark]]
		wantErrArg bool
	}{
		{
			name:       "Ok",
			pagination: models.Pagination{Limit: 10, Offset: 20},
			callRepo:   true,
			page:       method[models.Page[models.Mark]]{data: models.Page[models.Mark]{Items: []models.Mark{{}}, Total: 21}},
		},
		{
			name:       "ErrRepo",
			pagination: models.Pagination{Limit: 10},
			callRepo:   true,
			page:       method[models.Page[models.Mark]]{err: errors.New("")},
		},
		{
			name:       "ErrLimit",
			pagination: models.Pagination{Limit: models.MaxLimit + 1},
			wantErrArg: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callRepo {
				suite.marksRepo.On("GetMarksByUserId", mock.Anything, 1, tt.pagination).Once().
					Return(tt.page.data, tt.page.err)
			}

			got, gotErr := suite.uc.ListMarksByUserId(context.Background(), 1, tt.pagination)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(gotErr, usecase.ErrInvalidArgument)
			case tt.page.err != nil:
				suite.Error(gotErr)
			default:
				suite.NoError(gotErr)
				suite.Equal(tt.page.data, got)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkById() {
	tests := []struct {
		name        string
		getMarkById method[models.Mark]
	}{
		{
			name: "Ok",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkById.data, tt.getMarkById.err)
				if tt.getMarkById.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkById(context.Background(), 1)

			if tt.getMarkById.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarksByUserId() {
	tests := []struct {
		name             string
		getMarksByUserId method[models.Page[models.Mark]]
	}{
		{
			name: "Ok",
			getMarksByUserId: method[models.Page[models.Mark]]{
				data: models.Page[models.Mark]{Items: []models.Mark{}},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarksByUserId: method[models.Page[models.Mark]]{
				data: models.Page[models.Mark]{},
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarksByUserId", mock.Anything, mock.AnythingOfType("int"), models.Pagination{}).Once().
					Return(tt.getMarksByUserId.data, tt.getMarksByUserId.err)
				if tt.getMarksByUserId.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarksByUserId(context.Background(), 1)

			if tt.getMarksByUserId.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestAddMark() {
	tests := []struct {
		name                         string
		trDo                         method[any]
		addMark                      method[int64]
		getLastMarkStatusHistoryItem method[models.MarkStatusHistoryItem]
		addCheck                     method[int64]
		addPhotos                    method[any]
	}{
		{
			name: "Ok",
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: nil,
			},
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: nil,
			},
		},
		{
			name: "ErrAddMark",
			trDo: method[any]{
				err: errors.New(""),
			},
			addMark: method[int64]{
				data: int64(0),
				err:  errors.New(""),
			},
		},
		{
			name: "ErrGetLastMarkStatusHistoryItem",
			trDo: method[any]{
				err: errors.New(""),
			},
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: errors.New(""),
			},
		},
		{
			name: "ErrAddCheck",
			trDo: method[any]{
				err: errors.New(""),
			},
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: nil,
			},
			addCheck: method[int64]{
				data: int64(0),
				err:  errors.New(""),
			},
		},
		{
			name: "ErrAddPhotos",
			trDo: method[any]{
				err: errors.New(""),
			},
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: nil,
			},
			addCheck: method[int64]{
				data: int64(1),
				err:  nil,
			},
			addPhotos: method[any]{
				err: errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
					fn := args.Get(1).(func(ctx context.Context) error)
					ctx := args.Get(0).(context.Context)
					_ = fn(ctx)
				}).Return(tt.trDo.err)

				suite.marksRepo.On("AddMark", mock.Anything, mock.Anything).Once().
					Return(tt.addMark.data, tt.addMark.err)
				if tt.addMark.err != nil {
					return
				}

				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getLastMarkStatusHistoryItem.data, tt.getLastMarkStatusHistoryItem.err)
				if tt.getLastMarkStatusHistoryItem.err != nil {
					return
				}

				suite.checksRepo.On("AddCheck", mock.Anything, mock.Anything).Once().
					Return(tt.addCheck.data, tt.addCheck.err)
				if tt.addCheck.err != nil {
					return
				}

				suite.photosRepo.On("AddPhotos", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int"), mock.Anything).Once().
					Return(tt.addPhotos.err)
				if tt.addPhotos.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.AddMark(context.Background(), models.Mark{}, []io.Reader{})

			if tt.addMark.err == nil &&
				tt.getLastMarkStatusHistoryItem.err == nil &&
				tt.addCheck.err == nil &&
				tt.addPhotos.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkTypes() {
	tests := []struct {
		name         string
		getMarkTypes method[[]models.MarkType]
	}{
		{
			name: "Ok",
			getMarkTypes: method[[]models.MarkType]{
				data: []models.MarkType{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarkTypes: method[[]models.MarkType]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkTypes", mock.Anything).Once().
					Return(tt.getMarkTypes.data, tt.getMarkTypes.err)
				if tt.getMarkTypes.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkTypes(context.Background())

			if tt.getMarkTypes.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatuses() {
	tests := []struct {
		name            string
		getMarkStatuses method[[]models.MarkStatus]
	}{
		{
			name: "Ok",
			getMarkStatuses: method[[]models.MarkStatus]{
				data: []models.MarkStatus{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getMarkStatuses: method[[]models.MarkStatus]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkStatuses", mock.Anything).Once().
					Return(tt.getMarkStatuses.data, tt.getMarkStatuses.err)
				if tt.getMarkStatuses.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkStatuses(context.Background())

			if tt.getMarkStatuses.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkStatusHistoryByMarkId() {
	tests := []struct {
		name                         string
		getMarkStatusHistoryByMarkId method[[]models.MarkStatusHistoryItem]
		withChecks                   bool
		getChecksByMarkId            method[[]models.Check]
		getPhotosByMarkId            method[map[int]map[int][]string]
	}{
		{
			name: "Ok",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: false,
		},
		{
			name: "OkWithChecks",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				data: []models.MarkStatusHistoryItem{
					{ID: 1},
					{ID: 2},
				},
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				data: []models.Check{
					{ID: 1, MarkStatusHistoryItemId: 1},
					{ID: 2, MarkStatusHistoryItemId: 1},
				},
				err: nil,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				data: map[int]map[int][]string{
					1: {
						1: []string{"1", "2"},
					},
				},
				err: nil,
			},
		},
		{
			name: "ErrGetMarkStatusHistoryByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: errors.New(""),
			},
		},
		{
			name: "ErrGetChecksByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				err: errors.New(""),
			},
		},
		{
			name: "ErrGetPhotosByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				err: nil,
			},
			getPhotosByMarkId: method[map[int]map[int][]string]{
				err: errors.New(""),
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkStatusHistoryByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getMarkStatusHistoryByMarkId.data, tt.getMarkStatusHistoryByMarkId.err)
				if tt.getMarkStatusHistoryByMarkId.err != nil {
					return
				}

				if tt.withChecks {
					suite.checksRepo.On("GetChecksByMarkId", mock.Anything, mock.AnythingOfType("int"), models.Pagination{}).Once().
						Return(models.Page[models.Check]{Items: tt.getChecksByMarkId.data}, tt.getChecksByMarkId.err)
					if tt.getChecksByMarkId.err != nil {
						return
					}

					suite.photosRepo.On("GetPhotosByMarkId", mock.Anything, mock.AnythingOfType("int")).Once().
						Return(tt.getPhotosByMarkId.data, tt.getPhotosByMarkId.err)
					if tt.getPhotosByMarkId.err != nil {
						return
					}
				}
			}()

			_, gotErr := suite.uc.GetMarkStatusHistoryByMarkId(context.Background(), 1, tt.withChecks)

			if tt.getMarkStatusHistoryByMarkId.err == nil &&
				tt.getChecksByMarkId.err == nil &&
				tt.getPhotosByMarkId.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}
