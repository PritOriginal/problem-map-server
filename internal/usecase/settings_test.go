package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SettingsSuite struct {
	suite.Suite
	repo *usecase.MockSettingsRepository
	uc   *usecase.Settings
	now  time.Time
}

func TestSettings(t *testing.T) {
	suite.Run(t, new(SettingsSuite))
}

func (suite *SettingsSuite) SetupTest() {
	suite.repo = usecase.NewMockSettingsRepository(suite.T())
	suite.now = time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	suite.uc = usecase.NewSettings(slogdiscard.NewDiscardLogger(), usecase.DefaultRuntimeSettings(), suite.repo)
	suite.uc.SetNow(func() time.Time { return suite.now })
}

func (suite *SettingsSuite) stored(s usecase.RuntimeSettings) models.Setting {
	raw, err := json.Marshal(s)
	suite.Require().NoError(err)
	return models.Setting{Key: usecase.RuntimeSettingsKey, Value: raw, UpdatedAt: suite.now}
}

func (suite *SettingsSuite) TestValidate() {
	valid := usecase.DefaultRuntimeSettings()
	tests := []struct {
		name    string
		mutate  func(s *usecase.RuntimeSettings)
		wantErr bool
	}{
		{name: "defaults are valid"},
		{name: "vote threshold zero", mutate: func(s *usecase.RuntimeSettings) { s.VoteThreshold = 0 }, wantErr: true},
		{name: "vote threshold too big", mutate: func(s *usecase.RuntimeSettings) { s.VoteThreshold = usecase.MaxVoteThreshold + 1 }, wantErr: true},
		{name: "dedup radius negative", mutate: func(s *usecase.RuntimeSettings) { s.DedupRadiusM = -1 }, wantErr: true},
		{name: "dedup radius max", mutate: func(s *usecase.RuntimeSettings) { s.DedupRadiusM = usecase.MaxDedupRadiusM }},
		{name: "checks per day zero", mutate: func(s *usecase.RuntimeSettings) { s.MaxChecksPerDay = 0 }, wantErr: true},
		{name: "rating delta out of range", mutate: func(s *usecase.RuntimeSettings) { s.Rating.CheckWrong = -usecase.MaxRatingDelta - 1 }, wantErr: true},
		{name: "negative rating delta is fine", mutate: func(s *usecase.RuntimeSettings) { s.Rating.MarkRefuted = -100 }},
		{name: "tasks per user zero", mutate: func(s *usecase.RuntimeSettings) { s.Tasker.MaxTasksPerUser = 0 }, wantErr: true},
		{name: "required checks zero", mutate: func(s *usecase.RuntimeSettings) { s.Tasker.RequiredChecks = 0 }, wantErr: true},
		{name: "target probability above one", mutate: func(s *usecase.RuntimeSettings) { s.Tasker.TargetProbability = 1.5 }, wantErr: true},
		{name: "target probability zero", mutate: func(s *usecase.RuntimeSettings) { s.Tasker.TargetProbability = 0 }},
		{name: "radius zero", mutate: func(s *usecase.RuntimeSettings) { s.Tasker.MaxRadiusMeters = 0 }, wantErr: true},
		{name: "ttl too long", mutate: func(s *usecase.RuntimeSettings) { s.Tasker.TaskTTLHours = usecase.MaxTaskTTLHours + 1 }, wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			s := valid
			if tt.mutate != nil {
				tt.mutate(&s)
			}
			err := s.Validate()
			if tt.wantErr {
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			} else {
				suite.NoError(err)
			}
		})
	}
}

func (suite *SettingsSuite) TestRuntimeSettingsFromConfig() {
	cfg := &config.Config{
		Marks:  config.MarksConfig{DedupRadiusM: 120},
		Rating: config.RatingConfig{CheckCorrect: 5, CheckWrong: -3, MarkConfirmed: 7, MarkRefuted: -4, TaskCompleted: 2, MaxChecksPerDay: 9},
		Tasker: config.TaskerConfig{MaxTasksPerUser: 4, RequiredChecks: 3, TargetProbability: 0.9, MaxRadiusMeters: 7000, TaskTTL: 48 * time.Hour},
	}
	got := usecase.RuntimeSettingsFromConfig(cfg)

	suite.Equal(usecase.RuntimeSettings{
		VoteThreshold:   3,
		DedupRadiusM:    120,
		MaxChecksPerDay: 9,
		Rating:          usecase.RatingSettings{CheckCorrect: 5, CheckWrong: -3, MarkConfirmed: 7, MarkRefuted: -4, TaskCompleted: 2},
		Tasker:          usecase.TaskerSettings{MaxTasksPerUser: 4, RequiredChecks: 3, TargetProbability: 0.9, MaxRadiusMeters: 7000, TaskTTLHours: 48},
	}, got)
	suite.Equal(48*time.Hour, got.Tasker.TaskTTL())
}

func (suite *SettingsSuite) TestGet_DefaultsWhenNotStored() {
	suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().
		Return(models.Setting{}, repository.ErrNotFound)

	suite.Equal(usecase.DefaultRuntimeSettings(), suite.uc.Get(context.Background()))
}

func (suite *SettingsSuite) TestGet_CachesUntilTTL() {
	want := usecase.DefaultRuntimeSettings()
	want.VoteThreshold = 5
	suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().
		Return(suite.stored(want), nil)

	ctx := context.Background()
	suite.Equal(want, suite.uc.Get(ctx))
	// Inside the TTL the repository is not asked again.
	suite.now = suite.now.Add(usecase.SettingsCacheTTL - time.Second)
	suite.Equal(want, suite.uc.Get(ctx))

	// After the TTL the value is refreshed.
	want.VoteThreshold = 7
	suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().
		Return(suite.stored(want), nil)
	suite.now = suite.now.Add(2 * time.Second)
	suite.Equal(want, suite.uc.Get(ctx))
	suite.repo.AssertExpectations(suite.T())
}

func (suite *SettingsSuite) TestGet_KeepsLastKnownOnError() {
	want := usecase.DefaultRuntimeSettings()
	want.DedupRadiusM = 80
	suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().
		Return(suite.stored(want), nil)
	ctx := context.Background()
	suite.Equal(want, suite.uc.Get(ctx))

	suite.now = suite.now.Add(usecase.SettingsCacheTTL + time.Second)
	suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().
		Return(models.Setting{}, errors.New("db down"))
	suite.Equal(want, suite.uc.Get(ctx), "a failed refresh keeps the last known value")

	// The failure is cached for the TTL as well: no retry storm.
	suite.Equal(want, suite.uc.Get(ctx))
	suite.repo.AssertExpectations(suite.T())
}

func (suite *SettingsSuite) TestGet_PartialDocumentKeepsDefaults() {
	// A document written before tasker settings existed: the missing
	// section keeps the defaults.
	suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().
		Return(models.Setting{Value: json.RawMessage(`{"vote_threshold":4}`)}, nil)

	got := suite.uc.Get(context.Background())
	want := usecase.DefaultRuntimeSettings()
	want.VoteThreshold = 4
	suite.Equal(want, got)
}

func (suite *SettingsSuite) TestLoad() {
	tests := []struct {
		name    string
		setting models.Setting
		err     error
		want    usecase.RuntimeSettings
		wantErr error
	}{
		{name: "not stored", err: repository.ErrNotFound, want: usecase.DefaultRuntimeSettings()},
		{name: "stored", setting: models.Setting{Value: json.RawMessage(`{"max_checks_per_day":7}`)}, want: func() usecase.RuntimeSettings {
			s := usecase.DefaultRuntimeSettings()
			s.MaxChecksPerDay = 7
			return s
		}()},
		{name: "corrupt", setting: models.Setting{Value: json.RawMessage(`{"vote_threshold":0}`)}, wantErr: usecase.ErrInvalidArgument},
		{name: "db error", err: errRepo, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("GetSetting", mock.Anything, usecase.RuntimeSettingsKey).Once().Return(tt.setting, tt.err)

			got, err := suite.uc.Load(context.Background())
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.want, got)
		})
	}
}

func (suite *SettingsSuite) TestUpdate() {
	valid := usecase.DefaultRuntimeSettings()
	valid.VoteThreshold = 2
	invalid := valid
	invalid.MaxChecksPerDay = 0

	tests := []struct {
		name      string
		settings  usecase.RuntimeSettings
		updatedBy int
		wantBy    null.Int
		setErr    error
		wantErr   error
	}{
		{name: "ok", settings: valid, updatedBy: 42, wantBy: null.IntFrom(42)},
		{name: "unknown actor is stored as null", settings: valid, updatedBy: 0, wantBy: null.Int{}},
		{name: "invalid is rejected before writing", settings: invalid, updatedBy: 42, wantErr: usecase.ErrInvalidArgument},
		{name: "repository error", settings: valid, updatedBy: 42, wantBy: null.IntFrom(42), setErr: errRepo, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantErr == nil || tt.setErr != nil {
				raw, err := json.Marshal(tt.settings)
				suite.Require().NoError(err)
				suite.repo.On("SetSetting", mock.Anything, usecase.RuntimeSettingsKey, json.RawMessage(raw), tt.wantBy).Once().
					Return(tt.setErr)
			}

			got, err := suite.uc.Update(context.Background(), tt.settings, tt.updatedBy)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.settings, got)
			// The cache is refreshed at once: no read from the repository.
			suite.Equal(tt.settings, suite.uc.Get(context.Background()))
		})
	}
	suite.repo.AssertNotCalled(suite.T(), "GetSetting", mock.Anything, mock.Anything)
}

func (suite *SettingsSuite) TestHistory() {
	changes := []models.SettingChange{{ID: 2, Key: usecase.RuntimeSettingsKey}, {ID: 1, Key: usecase.RuntimeSettingsKey}}
	tests := []struct {
		name    string
		limit   int
		err     error
		wantErr error
	}{
		{name: "ok", limit: 20},
		{name: "limit zero", limit: 0, wantErr: usecase.ErrInvalidArgument},
		{name: "limit above max", limit: usecase.MaxSettingsHistoryLimit + 1, wantErr: usecase.ErrInvalidArgument},
		{name: "repository error", limit: 5, err: errRepo, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantErr == nil || tt.err != nil {
				suite.repo.On("GetSettingsHistory", mock.Anything, usecase.RuntimeSettingsKey, tt.limit).Once().Return(changes, tt.err)
			}
			got, err := suite.uc.History(context.Background(), tt.limit)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(changes, got)
		})
	}
}

func (suite *SettingsSuite) TestStaticSettings() {
	s := usecase.DefaultRuntimeSettings()
	s.VoteThreshold = 9
	suite.Equal(s, usecase.StaticSettings(s).Get(context.Background()))
}
