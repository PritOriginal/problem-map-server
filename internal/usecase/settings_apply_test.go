package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// SettingsApplySuite checks that the usecases read their thresholds from the
// SettingsProvider instead of the config.
type SettingsApplySuite struct {
	suite.Suite
	settings *usecase.MockSettingsProvider
}

func TestSettingsApply(t *testing.T) {
	suite.Run(t, new(SettingsApplySuite))
}

func (suite *SettingsApplySuite) SetupTest() {
	suite.settings = usecase.NewMockSettingsProvider(suite.T())
}

func (suite *SettingsApplySuite) TestUpdater_VoteThresholdFromSettings() {
	checks := []models.Check{{ID: 1, UserID: 2, Result: true}, {ID: 2, UserID: 3, Result: true}}
	tests := []struct {
		name        string
		threshold   int
		wantResolve bool
	}{
		{name: "score 2 resolves with threshold 2", threshold: 2, wantResolve: true},
		{name: "score 2 waits with the default threshold 3", threshold: 3},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			marks := usecase.NewMockMarksRepository(suite.T())
			checksRepo := usecase.NewMockChecksRepository(suite.T())
			users := usecase.NewMockUsersRepository(suite.T())
			settings := usecase.NewMockSettingsProvider(suite.T())

			s := usecase.DefaultRuntimeSettings()
			s.VoteThreshold = tt.threshold
			s.Rating = usecase.RatingSettings{CheckCorrect: 10, CheckWrong: -5, MarkConfirmed: 20, MarkRefuted: -7, TaskCompleted: 1}
			settings.On("Get", mock.Anything).Return(s)

			marks.On("GetMarkById", mock.Anything, 1).Once().Return(models.Mark{ID: 1, UserID: 9, MarkStatusID: models.UnconfirmedStatus}, nil)
			marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 1).Once().Return(models.MarkStatusHistoryItem{ID: 5}, nil)
			checksRepo.On("GetChecksByMarkHistoryId", mock.Anything, 5).Once().Return(checks, nil)
			if tt.wantResolve {
				marks.On("UpdateMarkStatus", mock.Anything, 1, models.ConfirmedStatus).Once().Return(nil)
				// The deltas come from the settings too: 10 per correct check,
				// 20 for the confirmed author.
				users.On("AddRatingEvent", mock.Anything, mock.MatchedBy(func(e models.RatingEvent) bool {
					return e.Reason == models.RatingReasonCheckCorrect && e.Delta == 10
				})).Times(len(checks)).Return(int64(1), nil)
				users.On("AddRatingEvent", mock.Anything, mock.MatchedBy(func(e models.RatingEvent) bool {
					return e.Reason == models.RatingReasonMarkConfirmed && e.Delta == 20 && e.UserID == 9
				})).Once().Return(int64(1), nil)
			}

			u := usecase.NewUpdater(slogdiscard.NewDiscardLogger(), ratingCfg, usecase.NewMockManager(suite.T()), usecase.UpdaterRepositories{
				Marks: marks, Checks: checksRepo, Users: users,
			}).WithSettings(settings)

			suite.NoError(u.Update(context.Background(), 1))
			marks.AssertExpectations(suite.T())
			users.AssertExpectations(suite.T())
		})
	}
}

func (suite *SettingsApplySuite) TestChecks_DailyLimitFromSettings() {
	tests := []struct {
		name    string
		limit   int
		already int
		wantErr error
	}{
		{name: "under the limit", limit: 5, already: 4},
		{name: "at the limit", limit: 5, already: 5, wantErr: usecase.ErrTooManyRequests},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			marks := usecase.NewMockMarksRepository(suite.T())
			checksRepo := usecase.NewMockChecksRepository(suite.T())
			settings := usecase.NewMockSettingsProvider(suite.T())
			s := usecase.DefaultRuntimeSettings()
			s.MaxChecksPerDay = tt.limit
			settings.On("Get", mock.Anything).Return(s)

			trm := usecase.NewMockManager(suite.T())
			trm.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
			marks.On("LockMark", mock.Anything, 1).Once().Return(nil)
			marks.On("GetMarkById", mock.Anything, 1).Once().Return(models.Mark{ID: 1, UserID: 9, MarkStatusID: models.UnconfirmedStatus}, nil)
			marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 1).Once().Return(models.MarkStatusHistoryItem{ID: 5}, nil)
			checksRepo.On("CountChecksByUserIdSince", mock.Anything, 2, mock.AnythingOfType("time.Time")).Once().Return(tt.already, nil)
			if tt.wantErr == nil {
				// The rest of AddCheck is not exercised here: the next lookup
				// fails and the error is returned as is.
				checksRepo.On("GetUserMarkCheck", mock.Anything, 2, 5).Once().Return(models.Check{}, errRepo)
			}

			uc := usecase.NewChecks(slogdiscard.NewDiscardLogger(), ratingCfg, trm, usecase.NewMockMarkStatusUpdater(suite.T()), usecase.ChecksRepositories{
				Marks: marks, Checks: checksRepo,
			}).WithSettings(settings)

			_, err := uc.AddCheck(context.Background(), models.Check{UserID: 2, MarkID: 1, CreatedAt: time.Now()}, nil)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.ErrorIs(err, errRepo)
			}
		})
	}
}

func (suite *SettingsApplySuite) TestMarks_DedupRadiusFromSettings() {
	marks := usecase.NewMockMarksRepository(suite.T())
	s := usecase.DefaultRuntimeSettings()
	s.DedupRadiusM = 250
	suite.settings.On("Get", mock.Anything).Return(s)

	marks.On("GetSimilarMarks", mock.Anything, models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7, MarkTypeID: 1, RadiusM: 250}).Once().
		Return([]models.MarkWithDistance{}, nil)

	uc := usecase.NewMarks(slogdiscard.NewDiscardLogger(), config.MarksConfig{DedupRadiusM: 50}, usecase.NewMockManager(suite.T()), usecase.MarksRepositories{Marks: marks}).
		WithSettings(suite.settings)
	_, err := uc.FindSimilarMarks(context.Background(), models.GetSimilarMarksFilters{Lon: 41.4, Lat: 52.7, MarkTypeID: 1})
	suite.NoError(err)
}
