package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type HealthSuite struct {
	suite.Suite
	cfg config.HealthConfig
}

func TestHealth(t *testing.T) {
	suite.Run(t, new(HealthSuite))
}

func (suite *HealthSuite) SetupSuite() {
	suite.cfg = config.HealthConfig{Timeout: time.Second, CacheTTL: time.Second}
}

type typedNilPinger struct{}

func (*typedNilPinger) Ping(context.Context) error { return nil }

func (suite *HealthSuite) TestCheck() {
	tests := []struct {
		name string
		deps func() HealthDependencies
		want HealthReport
		err  error
	}{
		{
			name: "AllOk",
			deps: func() HealthDependencies {
				pg, rd := NewMockPinger(suite.T()), NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(nil)
				rd.EXPECT().Ping(mock.Anything).Return(nil)
				return HealthDependencies{Required: map[string]Pinger{"postgres": pg}, Optional: map[string]Pinger{"redis": rd}}
			},
			want: HealthReport{"postgres": HealthStatusOK, "redis": HealthStatusOK},
		},
		{
			name: "RequiredFails",
			deps: func() HealthDependencies {
				pg, rd := NewMockPinger(suite.T()), NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(errors.New("down"))
				rd.EXPECT().Ping(mock.Anything).Return(nil)
				return HealthDependencies{Required: map[string]Pinger{"postgres": pg}, Optional: map[string]Pinger{"redis": rd}}
			},
			want: HealthReport{"postgres": HealthStatusError, "redis": HealthStatusOK},
			err:  ErrUnavailable,
		},
		{
			name: "OptionalFailsIsReportedOnly",
			deps: func() HealthDependencies {
				pg, rd := NewMockPinger(suite.T()), NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(nil)
				rd.EXPECT().Ping(mock.Anything).Return(errors.New("down"))
				return HealthDependencies{Required: map[string]Pinger{"postgres": pg}, Optional: map[string]Pinger{"redis": rd}}
			},
			want: HealthReport{"postgres": HealthStatusOK, "redis": HealthStatusError},
		},
		{
			name: "BothFail",
			deps: func() HealthDependencies {
				pg, rd := NewMockPinger(suite.T()), NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(errors.New("down"))
				rd.EXPECT().Ping(mock.Anything).Return(errors.New("down"))
				return HealthDependencies{Required: map[string]Pinger{"postgres": pg}, Optional: map[string]Pinger{"redis": rd}}
			},
			want: HealthReport{"postgres": HealthStatusError, "redis": HealthStatusError},
			err:  ErrUnavailable,
		},
		{
			name: "NilDependenciesSkipped",
			deps: func() HealthDependencies {
				pg := NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(nil)
				var typedNil *typedNilPinger
				return HealthDependencies{
					Required: map[string]Pinger{"postgres": pg, "redis": nil},
					Optional: map[string]Pinger{"s3": typedNil},
				}
			},
			want: HealthReport{"postgres": HealthStatusOK},
		},
		{
			name: "NoDependencies",
			deps: func() HealthDependencies { return HealthDependencies{} },
			want: HealthReport{},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			uc := NewHealth(slogdiscard.NewDiscardLogger(), suite.cfg, tt.deps())

			got, err := uc.Check(context.Background())

			suite.ErrorIs(err, tt.err)
			suite.Equal(tt.want, got)
		})
	}
}

// TestCheckCancelledContextNotCached: a caller whose context is already
// cancelled gets ctx.Err() and must not leave a poisoned entry in the cache.
func (suite *HealthSuite) TestCheckCancelledContextNotCached() {
	pg := NewMockPinger(suite.T())
	pg.EXPECT().Ping(mock.Anything).Return(nil).Once()
	uc := NewHealth(slogdiscard.NewDiscardLogger(), suite.cfg, HealthDependencies{Required: map[string]Pinger{"postgres": pg}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := uc.Check(ctx)
	suite.ErrorIs(err, context.Canceled)
	suite.Nil(got)

	got, err = uc.Check(context.Background())
	suite.NoError(err)
	suite.Equal(HealthReport{"postgres": HealthStatusOK}, got)
}

// TestCheckCallerCancelledMidPing: a caller that gives up while the ping is
// running gets ctx.Err(); the ping still completes and its real result is
// what other callers see.
func (suite *HealthSuite) TestCheckCallerCancelledMidPing() {
	release := make(chan struct{})
	pg := NewMockPinger(suite.T())
	pg.EXPECT().Ping(mock.Anything).RunAndReturn(func(context.Context) error {
		<-release
		return nil
	}).Once()
	uc := NewHealth(slogdiscard.NewDiscardLogger(), suite.cfg, HealthDependencies{Required: map[string]Pinger{"postgres": pg}})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	got, err := uc.Check(ctx)
	suite.ErrorIs(err, context.Canceled)
	suite.Nil(got)

	close(release)
	got, err = uc.Check(context.Background())
	suite.NoError(err)
	suite.Equal(HealthReport{"postgres": HealthStatusOK}, got)
}

// TestCheckSingleFlight: concurrent callers wait for one ping rather than
// running their own in turn.
func (suite *HealthSuite) TestCheckSingleFlight() {
	const callers = 8

	release := make(chan struct{})
	pg := NewMockPinger(suite.T())
	pg.EXPECT().Ping(mock.Anything).RunAndReturn(func(context.Context) error {
		<-release
		return errors.New("down")
	}).Once()
	uc := NewHealth(slogdiscard.NewDiscardLogger(), config.HealthConfig{Timeout: time.Second}, HealthDependencies{Required: map[string]Pinger{"postgres": pg}})

	var (
		started sync.WaitGroup
		done    sync.WaitGroup
	)
	results := make([]error, callers)
	for i := range callers {
		started.Add(1)
		done.Add(1)
		go func() {
			defer done.Done()
			started.Done()
			_, results[i] = uc.Check(context.Background())
		}()
	}
	started.Wait()
	time.Sleep(20 * time.Millisecond) // let every caller reach the select
	close(release)
	done.Wait()

	for _, err := range results {
		suite.ErrorIs(err, ErrUnavailable)
	}
}

func (suite *HealthSuite) TestCheckCachesResult() {
	pg := NewMockPinger(suite.T())
	pg.EXPECT().Ping(mock.Anything).Return(nil).Once()
	uc := NewHealth(slogdiscard.NewDiscardLogger(), suite.cfg, HealthDependencies{Required: map[string]Pinger{"postgres": pg}})

	for range 3 {
		got, err := uc.Check(context.Background())
		suite.NoError(err)
		suite.Equal(HealthReport{"postgres": HealthStatusOK}, got)
	}
}
