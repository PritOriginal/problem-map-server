package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
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

func (suite *MarksSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.marksRepo = usecase.NewMockMarksRepository(suite.T())
	suite.checksRepo = usecase.NewMockChecksRepository(suite.T())
	suite.photosRepo = usecase.NewMockPhotosRepository(suite.T())
	suite.uc = usecase.NewMarks(suite.log, config.MarksConfig{DedupRadiusM: 50}, suite.trManager, usecase.MarksRepositories{
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
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// GetMarks must always ask the repository for the full set.
			suite.marksRepo.On("GetMarks", mock.Anything, mock.MatchedBy(func(f models.GetMarksFilters) bool {
				return f.Pagination == models.Pagination{}
			})).Once().Return(tt.getMarks.data, tt.getMarks.err)

			got, gotErr := suite.uc.GetMarks(context.Background(), models.GetMarksFilters{
				Pagination: models.Pagination{Limit: 10, Offset: 5},
			})

			if tt.getMarks.err == nil {
				suite.NoError(gotErr)
				suite.Equal(tt.getMarks.data.Items, got)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getMarks.err)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestGetMarkChanges() {
	since := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		filters    models.MarkChangesFilters
		getMarks   method[models.Page[models.Mark]]
		getDeleted method[models.Page[int]]
		wantErrArg bool
	}{
		{
			name:       "Ok",
			filters:    models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: 50}},
			getMarks:   method[models.Page[models.Mark]]{data: models.Page[models.Mark]{Items: []models.Mark{{ID: 1}}, Total: 7}},
			getDeleted: method[models.Page[int]]{data: models.Page[int]{Items: []int{4, 5}, Total: 9}},
		},
		{
			name:       "ErrSinceRequired",
			filters:    models.MarkChangesFilters{Pagination: models.Pagination{Limit: 50}},
			wantErrArg: true,
		},
		{
			name:       "ErrSinceInFuture",
			filters:    models.MarkChangesFilters{Since: time.Now().Add(time.Hour), Pagination: models.Pagination{Limit: 50}},
			wantErrArg: true,
		},
		{
			name:       "ErrPagination",
			filters:    models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: 501}},
			wantErrArg: true,
		},
		{
			name:     "ErrGetMarks",
			filters:  models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: 50}},
			getMarks: method[models.Page[models.Mark]]{err: errRepo},
		},
		{
			name:       "ErrGetDeleted",
			filters:    models.MarkChangesFilters{Since: since, Pagination: models.Pagination{Limit: 50}},
			getMarks:   method[models.Page[models.Mark]]{data: models.Page[models.Mark]{}},
			getDeleted: method[models.Page[int]]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrArg {
				suite.marksRepo.On("GetMarks", mock.Anything, models.GetMarksFilters{
					UpdatedSince: since,
					Sort:         models.MarksSortUpdatedAt,
					Order:        models.SortAsc,
					Pagination:   tt.filters.Pagination,
				}).Once().Return(tt.getMarks.data, tt.getMarks.err)
				if tt.getMarks.err == nil {
					suite.marksRepo.On("GetDeletedMarkIDs", mock.Anything, since, tt.filters.Pagination).Once().
						Return(tt.getDeleted.data, tt.getDeleted.err)
				}
			}

			before := time.Now()
			got, err := suite.uc.GetMarkChanges(context.Background(), tt.filters)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			case tt.getMarks.err != nil:
				assertRepoErr(&suite.Suite, err, tt.getMarks.err)
			case tt.getDeleted.err != nil:
				assertRepoErr(&suite.Suite, err, tt.getDeleted.err)
			default:
				suite.Require().NoError(err)
				suite.Equal(tt.getMarks.data.Items, got.Marks)
				suite.Equal(tt.getMarks.data.Total, got.Total)
				suite.Equal(tt.getDeleted.data.Items, got.DeletedIDs)
				suite.Equal(tt.getDeleted.data.Total, got.DeletedTotal)
				suite.NotNil(got.HiddenIDs)
				suite.Empty(got.HiddenIDs)
				suite.False(got.ServerTime.Before(before.UTC().Truncate(time.Second)))
				suite.Equal(time.UTC, got.ServerTime.Location())
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
			getMarks: method[models.Page[models.Mark]]{err: errRepo},
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
			nearby:   method[models.Page[models.MarkWithDistance]]{err: errRepo},
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
			name:       "ErrLongitudeNaN",
			filters:    models.GetMarksNearbyFilters{Lon: math.NaN(), Lat: 52.72, RadiusM: 100},
			wantErrArg: true,
		},
		{
			name:       "ErrRadiusNaN",
			filters:    models.GetMarksNearbyFilters{Lon: 41.45, Lat: 52.72, RadiusM: math.NaN()},
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
			page:       method[models.Page[models.Mark]]{err: errRepo},
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
			name: "ErrRepo",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  errRepo,
			},
		},
		{
			name: "ErrNotFound",
			getMarkById: method[models.Mark]{
				data: models.Mark{},
				err:  repository.ErrNotFound,
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
				assertRepoErr(&suite.Suite, gotErr, tt.getMarkById.err)
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
				err:  errRepo,
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
				assertRepoErr(&suite.Suite, gotErr, tt.getMarksByUserId.err)
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
				err: errRepo,
			},
			addMark: method[int64]{
				data: int64(0),
				err:  errRepo,
			},
		},
		{
			name: "ErrGetLastMarkStatusHistoryItem",
			trDo: method[any]{
				err: errRepo,
			},
			addMark: method[int64]{
				data: int64(1),
				err:  nil,
			},
			getLastMarkStatusHistoryItem: method[models.MarkStatusHistoryItem]{
				err: errRepo,
			},
		},
		{
			name: "ErrAddCheck",
			trDo: method[any]{
				err: errRepo,
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
				err:  errRepo,
			},
		},
		{
			name: "ErrAddPhotos",
			trDo: method[any]{
				err: errRepo,
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
				err: errRepo,
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

				suite.marksRepo.On("FollowMark", mock.Anything, 7, int(tt.addMark.data)).Once().Return(nil)

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

			_, gotErr := suite.uc.AddMark(context.Background(), models.Mark{UserID: 7}, []io.Reader{}, true)

			if tt.addMark.err == nil &&
				tt.getLastMarkStatusHistoryItem.err == nil &&
				tt.addCheck.err == nil &&
				tt.addPhotos.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.addMark.err, tt.getLastMarkStatusHistoryItem.err, tt.addCheck.err, tt.addPhotos.err)
			}
			suite.marksRepo.AssertExpectations(suite.T())
			suite.checksRepo.AssertExpectations(suite.T())
			suite.photosRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksSuite) TestAddMarkDedup() {
	newMark := models.Mark{Geom: models.NewPoint(geom.Coord{41.44, 52.72}), MarkTypeID: 3, UserID: 7}
	similar := []models.MarkWithDistance{{Mark: models.Mark{ID: 5, MarkTypeID: 3}, DistanceM: 12.5}}

	tests := []struct {
		name        string
		mark        models.Mark
		force       bool
		similar     method[[]models.MarkWithDistance]
		wantCreate  bool
		wantSimilar bool
		wantErrArg  bool
	}{
		{name: "NoSimilarCreates", mark: newMark, similar: method[[]models.MarkWithDistance]{data: []models.MarkWithDistance{}}, wantCreate: true},
		{name: "SimilarConflict", mark: newMark, similar: method[[]models.MarkWithDistance]{data: similar}, wantSimilar: true},
		{name: "SimilarForced", mark: newMark, force: true, wantCreate: true},
		{name: "ErrSearch", mark: newMark, similar: method[[]models.MarkWithDistance]{err: errRepo}},
		{name: "ErrNoGeom", mark: models.Mark{MarkTypeID: 3, UserID: 7}, wantErrArg: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.force && !tt.wantErrArg {
				// The search must use the mark's point, type and the configured radius.
				suite.marksRepo.On("GetSimilarMarks", mock.Anything, models.GetSimilarMarksFilters{
					Lon: 41.44, Lat: 52.72, MarkTypeID: 3, RadiusM: 50,
				}).Once().Return(tt.similar.data, tt.similar.err)
			}
			if tt.wantCreate {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
					_ = args.Get(1).(func(ctx context.Context) error)(args.Get(0).(context.Context))
				}).Return(nil)
				suite.marksRepo.On("AddMark", mock.Anything, tt.mark).Once().Return(int64(9), nil)
				suite.marksRepo.On("FollowMark", mock.Anything, 7, 9).Once().Return(nil)
				suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, 9).Once().Return(models.MarkStatusHistoryItem{ID: 3}, nil)
				suite.checksRepo.On("AddCheck", mock.Anything, mock.Anything).Once().Return(int64(1), nil)
				suite.photosRepo.On("AddPhotos", mock.Anything, 9, 1, mock.Anything).Once().Return(nil)
			}

			id, err := suite.uc.AddMark(context.Background(), tt.mark, nil, tt.force)

			switch {
			case tt.wantCreate:
				suite.NoError(err)
				suite.Equal(int64(9), id)
			case tt.wantSimilar:
				var similarErr *usecase.SimilarMarksError
				suite.ErrorAs(err, &similarErr)
				suite.ErrorIs(err, usecase.ErrConflict)
				suite.Equal(similar, similarErr.Marks)
			case tt.wantErrArg:
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			default:
				assertRepoErr(&suite.Suite, err, tt.similar.err)
			}
		})
	}
}

func (suite *MarksSuite) TestFindSimilarMarks() {
	tests := []struct {
		name       string
		filters    models.GetSimilarMarksFilters
		wantRadius float64
		similar    method[[]models.MarkWithDistance]
		wantErrArg bool
	}{
		{name: "Ok", filters: models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7, MarkTypeID: 1, RadiusM: 100}, wantRadius: 100,
			similar: method[[]models.MarkWithDistance]{data: []models.MarkWithDistance{{}}}},
		{name: "DefaultRadius", filters: models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7, MarkTypeID: 1}, wantRadius: 50},
		{name: "ErrRepo", filters: models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7, MarkTypeID: 1}, wantRadius: 50,
			similar: method[[]models.MarkWithDistance]{err: errRepo}},
		{name: "ErrRadiusTooBig", filters: models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7, MarkTypeID: 1, RadiusM: models.MaxNearbyRadiusM + 1}, wantErrArg: true},
		{name: "ErrNoType", filters: models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7}, wantErrArg: true},
		{name: "ErrBadLon", filters: models.GetSimilarMarksFilters{Lon: 181, Lat: 52.7, MarkTypeID: 1}, wantErrArg: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrArg {
				want := tt.filters
				want.RadiusM = tt.wantRadius
				suite.marksRepo.On("GetSimilarMarks", mock.Anything, want).Once().Return(tt.similar.data, tt.similar.err)
			}

			got, err := suite.uc.FindSimilarMarks(context.Background(), tt.filters)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			case tt.similar.err != nil:
				assertRepoErr(&suite.Suite, err, tt.similar.err)
			default:
				suite.NoError(err)
				suite.Equal(tt.similar.data, got)
			}
		})
	}
}

var (
	actorOwner     = models.Actor{UserID: 1, Role: models.RoleUser}
	actorStranger  = models.Actor{UserID: 2, Role: models.RoleUser}
	actorModerator = models.Actor{UserID: 3, Role: models.RoleModerator}
)

func (suite *MarksSuite) TestUpdateMark() {
	desc := "new description"
	upd := models.MarkUpdate{Description: &desc}
	unconfirmed := models.Mark{ID: 10, UserID: 1, MarkStatusID: models.UnconfirmedStatus}
	confirmed := models.Mark{ID: 10, UserID: 1, MarkStatusID: models.ConfirmedStatus}

	tests := []struct {
		name       string
		actor      models.Actor
		upd        models.MarkUpdate
		getMark    method[models.Mark]
		wantUpdate bool
		update     error
		wantErr    error
		wantErrArg bool
	}{
		{name: "OwnerUnconfirmed", actor: actorOwner, upd: upd, getMark: method[models.Mark]{data: unconfirmed}, wantUpdate: true},
		{name: "ModeratorAnyStatus", actor: actorModerator, upd: upd, getMark: method[models.Mark]{data: confirmed}, wantUpdate: true},
		{name: "OwnerConfirmed409", actor: actorOwner, upd: upd, getMark: method[models.Mark]{data: confirmed}, wantErr: usecase.ErrConflict},
		{name: "Stranger403", actor: actorStranger, upd: upd, getMark: method[models.Mark]{data: unconfirmed}, wantErr: usecase.ErrForbidden},
		{name: "NotFound", actor: actorOwner, upd: upd, getMark: method[models.Mark]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "EmptyUpdate", actor: actorOwner, wantErrArg: true},
		{name: "ErrUpdate", actor: actorOwner, upd: upd, getMark: method[models.Mark]{data: unconfirmed}, wantUpdate: true, update: errRepo, wantErr: errRepo},
		{name: "UnknownType400", actor: actorOwner, upd: upd, getMark: method[models.Mark]{data: unconfirmed}, wantUpdate: true, update: repository.ErrInvalidReference, wantErr: usecase.ErrInvalidArgument},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrArg {
				suite.marksRepo.On("GetMarkById", mock.Anything, 10).Once().Return(tt.getMark.data, tt.getMark.err)
			}
			if tt.wantUpdate {
				suite.marksRepo.On("UpdateMark", mock.Anything, 10, tt.upd).Once().Return(tt.update)
				if tt.update == nil {
					updated := tt.getMark.data
					updated.Description = desc
					suite.marksRepo.On("GetMarkById", mock.Anything, 10).Once().Return(updated, nil)
				}
			}

			got, err := suite.uc.UpdateMark(context.Background(), tt.actor, 10, tt.upd)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			default:
				suite.NoError(err)
				suite.Equal(desc, got.Description)
			}
		})
	}
}

func (suite *MarksSuite) TestDeleteMark() {
	unconfirmed := models.Mark{ID: 10, UserID: 1, MarkStatusID: models.UnconfirmedStatus}
	confirmed := models.Mark{ID: 10, UserID: 1, MarkStatusID: models.ConfirmedStatus}
	ownChecks := []models.Check{{UserID: 1}}
	foreignChecks := []models.Check{{UserID: 1}, {UserID: 2}}

	tests := []struct {
		name         string
		actor        models.Actor
		getMark      method[models.Mark]
		wantChecks   bool
		checks       method[[]models.Check]
		wantDelete   bool
		deleteMark   error
		deletePhotos error
		wantErr      error
	}{
		{name: "OwnerNoForeignChecks", actor: actorOwner, getMark: method[models.Mark]{data: unconfirmed}, wantChecks: true, checks: method[[]models.Check]{data: ownChecks}, wantDelete: true},
		{name: "OwnerForeignChecks409", actor: actorOwner, getMark: method[models.Mark]{data: unconfirmed}, wantChecks: true, checks: method[[]models.Check]{data: foreignChecks}, wantErr: usecase.ErrConflict},
		{name: "OwnerConfirmed409", actor: actorOwner, getMark: method[models.Mark]{data: confirmed}, wantErr: usecase.ErrConflict},
		{name: "Stranger403", actor: actorStranger, getMark: method[models.Mark]{data: unconfirmed}, wantErr: usecase.ErrForbidden},
		{name: "ModeratorSkipsCheckRule", actor: actorModerator, getMark: method[models.Mark]{data: confirmed}, wantDelete: true},
		{name: "NotFound", actor: actorOwner, getMark: method[models.Mark]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "ErrChecks", actor: actorOwner, getMark: method[models.Mark]{data: unconfirmed}, wantChecks: true, checks: method[[]models.Check]{err: errRepo}, wantErr: errRepo},
		{name: "ErrDeleteMark", actor: actorModerator, getMark: method[models.Mark]{data: confirmed}, wantDelete: true, deleteMark: errRepo, wantErr: errRepo},
		// Photos are removed after the commit; a storage failure is only logged.
		{name: "ErrDeletePhotosIgnored", actor: actorModerator, getMark: method[models.Mark]{data: confirmed}, wantDelete: true, deletePhotos: errRepo},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.marksRepo.On("GetMarkById", mock.Anything, 10).Once().Return(tt.getMark.data, tt.getMark.err)
			if tt.wantChecks {
				suite.checksRepo.On("GetChecksByMarkId", mock.Anything, 10, models.Pagination{}).Once().
					Return(models.Page[models.Check]{Items: tt.checks.data}, tt.checks.err)
			}
			if tt.wantDelete {
				var txErr error
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
					txErr = args.Get(1).(func(ctx context.Context) error)(args.Get(0).(context.Context))
				}).Return(func(context.Context, func(context.Context) error) error { return txErr })
				suite.marksRepo.On("DeleteMark", mock.Anything, 10).Once().Return(tt.deleteMark)
				if tt.deleteMark == nil {
					suite.photosRepo.On("DeletePhotos", mock.Anything, 10).Once().Return(tt.deletePhotos)
				}
			}

			err := suite.uc.DeleteMark(context.Background(), tt.actor, 10)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *MarksSuite) TestFollowMark() {
	tests := []struct {
		name    string
		follow  error
		wantErr error
	}{
		{name: "Ok"},
		{name: "NotFound", follow: repository.ErrNotFound, wantErr: usecase.ErrNotFound},
		{name: "ErrRepo", follow: errRepo, wantErr: errRepo},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.marksRepo.On("FollowMark", mock.Anything, 1, 10).Once().Return(tt.follow)

			err := suite.uc.FollowMark(context.Background(), 1, 10)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *MarksSuite) TestUnfollowMark() {
	tests := []struct {
		name     string
		getMark  method[models.Mark]
		unfollow error
		wantErr  error
	}{
		{name: "Ok"},
		{name: "NotFound", getMark: method[models.Mark]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "ErrRepo", unfollow: errRepo, wantErr: errRepo},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.marksRepo.On("GetMarkById", mock.Anything, 10).Once().Return(tt.getMark.data, tt.getMark.err)
			if tt.getMark.err == nil {
				suite.marksRepo.On("UnfollowMark", mock.Anything, 1, 10).Once().Return(tt.unfollow)
			}

			err := suite.uc.UnfollowMark(context.Background(), 1, 10)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *MarksSuite) TestListFollowedMarks() {
	tests := []struct {
		name       string
		p          models.Pagination
		page       method[models.Page[models.Mark]]
		wantErrArg bool
	}{
		{name: "Ok", p: models.Pagination{Limit: 10}, page: method[models.Page[models.Mark]]{data: models.Page[models.Mark]{Items: []models.Mark{{ID: 1}}, Total: 1}}},
		{name: "ErrRepo", p: models.Pagination{Limit: 10}, page: method[models.Page[models.Mark]]{err: errRepo}},
		{name: "ErrPagination", p: models.Pagination{Limit: models.MaxLimit + 1}, wantErrArg: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrArg {
				suite.marksRepo.On("GetFollowedMarks", mock.Anything, 1, tt.p).Once().Return(tt.page.data, tt.page.err)
			}

			got, err := suite.uc.ListFollowedMarks(context.Background(), 1, tt.p)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			case tt.page.err != nil:
				assertRepoErr(&suite.Suite, err, tt.page.err)
			default:
				suite.NoError(err)
				suite.Equal(tt.page.data, got)
			}
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
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkTypes", mock.Anything, models.LangEN).Once().
					Return(tt.getMarkTypes.data, tt.getMarkTypes.err)
				if tt.getMarkTypes.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkTypes(context.Background(), models.LangEN)

			if tt.getMarkTypes.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getMarkTypes.err)
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
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.marksRepo.On("GetMarkStatuses", mock.Anything, models.LangEN).Once().
					Return(tt.getMarkStatuses.data, tt.getMarkStatuses.err)
				if tt.getMarkStatuses.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetMarkStatuses(context.Background(), models.LangEN)

			if tt.getMarkStatuses.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getMarkStatuses.err)
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
				err: errRepo,
			},
		},
		{
			name: "ErrGetChecksByMarkId",
			getMarkStatusHistoryByMarkId: method[[]models.MarkStatusHistoryItem]{
				err: nil,
			},
			withChecks: true,
			getChecksByMarkId: method[[]models.Check]{
				err: errRepo,
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
				err: errRepo,
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
				assertRepoErr(&suite.Suite, gotErr, tt.getMarkStatusHistoryByMarkId.err, tt.getChecksByMarkId.err, tt.getPhotosByMarkId.err)
			}
			suite.marksRepo.AssertExpectations(suite.T())
		})
	}
}
