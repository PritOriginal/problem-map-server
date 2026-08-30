package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type ExportSuite struct {
	suite.Suite
	uc    *usecase.Export
	marks *usecase.MockExportMarksRepository
}

func (suite *ExportSuite) SetupTest() {
	suite.marks = usecase.NewMockExportMarksRepository(suite.T())
	suite.uc = usecase.NewExport(slogdiscard.NewDiscardLogger(), config.ExportConfig{MaxRows: 3}, usecase.ExportRepositories{
		Marks: suite.marks,
	})
}

func TestExport(t *testing.T) {
	suite.Run(t, new(ExportSuite))
}

func (suite *ExportSuite) TestExportMarks() {
	errBoom := errors.New("boom")
	errStop := errors.New("stop")
	marks := []models.Mark{{ID: 1}, {ID: 2}}

	tests := []struct {
		name     string
		filters  models.GetMarksFilters
		count    int
		countErr error
		iterErr  error
		fnErr    error
		wantIDs  []int
		wantErr  error
	}{
		{name: "Ok", filters: models.GetMarksFilters{MarkTypeIds: []int{1}, Pagination: models.Pagination{Limit: 7, Offset: 3}}, count: 2, wantIDs: []int{1, 2}},
		{name: "OkAtCap", count: 3, wantIDs: []int{1, 2}},
		{name: "ErrTooLarge", count: 4, wantErr: usecase.ErrExportTooLarge},
		{name: "ErrInvalidFilters", filters: models.GetMarksFilters{Sort: "nope"}, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrCount", countErr: errBoom, wantErr: errBoom},
		{name: "ErrIterate", count: 1, iterErr: errBoom, wantErr: errBoom},
		{name: "FnErrorStopsIteration", count: 2, fnErr: errStop, wantIDs: []int{1}, wantErr: errStop},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			validFilters := tt.filters.Sort.Validate() == nil
			if validFilters {
				// Pagination is dropped before the count and the cap becomes the limit.
				wantCount := tt.filters
				wantCount.Pagination = models.Pagination{}
				suite.marks.On("CountMarks", mock.Anything, wantCount).Once().Return(tt.count, tt.countErr)
			}
			if validFilters && tt.countErr == nil && tt.count <= 3 {
				wantIter := tt.filters
				wantIter.Pagination = models.Pagination{Limit: 3}
				suite.marks.On("IterateMarks", mock.Anything, wantIter, mock.Anything).Once().
					Return(func(_ context.Context, _ models.GetMarksFilters, fn func(models.Mark) error) error {
						if tt.iterErr != nil {
							return tt.iterErr
						}
						for _, m := range marks {
							if err := fn(m); err != nil {
								return err
							}
						}
						return nil
					})
			}

			var got []int
			err := suite.uc.ExportMarks(context.Background(), tt.filters, func(m models.Mark) error {
				got = append(got, m.ID)
				if tt.fnErr != nil {
					return tt.fnErr
				}
				return nil
			})
			suite.Equal(tt.wantIDs, got)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *ExportSuite) TestMaxRowsDefault() {
	uc := usecase.NewExport(slogdiscard.NewDiscardLogger(), config.ExportConfig{}, usecase.ExportRepositories{})
	suite.Equal(50_000, uc.MaxRows())
	suite.Equal(3, suite.uc.MaxRows())
}
