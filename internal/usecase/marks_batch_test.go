package usecase_test

import (
	"context"
	"errors"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/stretchr/testify/mock"
	"github.com/twpayne/go-geom"
)

// batchItem builds one batch element at the given point.
func batchItem(lon, lat float64, force bool) models.NewMark {
	return models.NewMark{
		Mark:  models.Mark{Geom: models.NewPoint(geom.Coord{lon, lat}), MarkTypeID: 3, UserID: 7},
		Force: force,
	}
}

// expectCreate arranges the transactional creation of a mark with the id.
func (suite *MarksSuite) expectCreate(markId int) {
	suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Run(func(args mock.Arguments) {
		_ = args.Get(1).(func(ctx context.Context) error)(args.Get(0).(context.Context))
	}).Return(nil)
	suite.marksRepo.On("AddMark", mock.Anything, mock.Anything).Once().Return(int64(markId), nil)
	suite.marksRepo.On("FollowMark", mock.Anything, 7, markId).Once().Return(nil)
	suite.marksRepo.On("GetLastMarkStatusHistoryItem", mock.Anything, markId).Once().
		Return(models.MarkStatusHistoryItem{ID: 3}, nil)
	suite.checksRepo.On("AddCheck", mock.Anything, mock.Anything).Once().Return(int64(1), nil)
	suite.photosRepo.On("AddPhotos", mock.Anything, markId, 1, mock.Anything).Once().Return(nil)
}

// TestAddMarksDedupInsideBatch: the items are applied in order, so the
// similarity search of the second item already sees what the first created.
func (suite *MarksSuite) TestAddMarksDedupInsideBatch() {
	similar := []models.MarkWithDistance{{Mark: models.Mark{ID: 11, MarkTypeID: 3}, DistanceM: 4}}

	// First item: nothing nearby yet, created as 11.
	suite.marksRepo.On("GetSimilarMarks", mock.Anything, models.GetSimilarMarksFilters{
		Lon: 41.44, Lat: 52.72, MarkTypeID: 3, RadiusM: 50,
	}).Once().Return([]models.MarkWithDistance{}, nil)
	suite.expectCreate(11)
	// Second item, a few meters away: the mark just created is returned.
	suite.marksRepo.On("GetSimilarMarks", mock.Anything, models.GetSimilarMarksFilters{
		Lon: 41.4401, Lat: 52.7201, MarkTypeID: 3, RadiusM: 50,
	}).Once().Return(similar, nil)

	results := suite.uc.AddMarks(context.Background(), []models.NewMark{
		batchItem(41.44, 52.72, false),
		batchItem(41.4401, 52.7201, false),
	})

	suite.Require().Len(results, 2)
	suite.Equal(models.BatchStatusCreated, results[0].Status)
	suite.Equal(int64(11), results[0].MarkID)
	suite.Equal(models.BatchStatusDuplicate, results[1].Status)
	suite.Equal(similar, results[1].SimilarMarks)
	var similarErr *usecase.SimilarMarksError
	suite.ErrorAs(results[1].Err, &similarErr)
}

// TestAddMarksFailureDoesNotAbort: a failing item is reported and the batch
// keeps going, in order.
func (suite *MarksSuite) TestAddMarksFailureDoesNotAbort() {
	var order []float64
	collect := func(args mock.Arguments) {
		order = append(order, args.Get(1).(models.GetSimilarMarksFilters).Lon)
	}

	suite.marksRepo.On("GetSimilarMarks", mock.Anything, mock.MatchedBy(func(f models.GetSimilarMarksFilters) bool {
		return f.Lon == 41.1
	})).Once().Run(collect).Return([]models.MarkWithDistance{}, nil)
	suite.expectCreate(21)

	// The second item fails in the repository.
	suite.marksRepo.On("GetSimilarMarks", mock.Anything, mock.MatchedBy(func(f models.GetSimilarMarksFilters) bool {
		return f.Lon == 41.2
	})).Once().Run(collect).Return(nil, errors.New("boom"))

	suite.marksRepo.On("GetSimilarMarks", mock.Anything, mock.MatchedBy(func(f models.GetSimilarMarksFilters) bool {
		return f.Lon == 41.3
	})).Once().Run(collect).Return([]models.MarkWithDistance{}, nil)
	suite.expectCreate(23)

	results := suite.uc.AddMarks(context.Background(), []models.NewMark{
		batchItem(41.1, 52.7, false),
		batchItem(41.2, 52.7, false),
		batchItem(41.3, 52.7, false),
	})

	suite.Require().Len(results, 3)
	suite.Equal(models.BatchStatusCreated, results[0].Status)
	suite.Equal(int64(21), results[0].MarkID)
	suite.Equal(models.BatchStatusFailed, results[1].Status)
	suite.Error(results[1].Err)
	suite.Equal(models.BatchStatusCreated, results[2].Status)
	suite.Equal(int64(23), results[2].MarkID)
	// The order of application is the order of the items.
	suite.Equal([]float64{41.1, 41.2, 41.3}, order)
}

// TestAddMarksForcedSkipsDedup: a forced item is created without a search,
// which is what a client repeating a rejected item sends.
func (suite *MarksSuite) TestAddMarksForcedSkipsDedup() {
	suite.expectCreate(31)

	results := suite.uc.AddMarks(context.Background(), []models.NewMark{batchItem(41.44, 52.72, true)})

	suite.Require().Len(results, 1)
	suite.Equal(models.BatchStatusCreated, results[0].Status)
	suite.Equal(int64(31), results[0].MarkID)
}

// TestAddMarksEmpty: nothing to apply, no repository calls.
func (suite *MarksSuite) TestAddMarksEmpty() {
	suite.Empty(suite.uc.AddMarks(context.Background(), nil))
}

// TestAddMarksCanceledContext: a canceled context stops the work but still
// reports one result per item.
func (suite *MarksSuite) TestAddMarksCanceledContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := suite.uc.AddMarks(ctx, []models.NewMark{
		batchItem(41.1, 52.7, false),
		batchItem(41.2, 52.7, false),
	})

	suite.Require().Len(results, 2)
	for _, res := range results {
		suite.Equal(models.BatchStatusFailed, res.Status)
		suite.ErrorIs(res.Err, context.Canceled)
	}
}
