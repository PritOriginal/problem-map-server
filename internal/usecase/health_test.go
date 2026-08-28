package usecase

import (
	"context"
	"errors"
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
				return HealthDependencies{"postgres": pg, "redis": rd}
			},
			want: HealthReport{"postgres": HealthStatusOK, "redis": HealthStatusOK},
		},
		{
			name: "OneFails",
			deps: func() HealthDependencies {
				pg, rd := NewMockPinger(suite.T()), NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(errors.New("down"))
				rd.EXPECT().Ping(mock.Anything).Return(nil)
				return HealthDependencies{"postgres": pg, "redis": rd}
			},
			want: HealthReport{"postgres": HealthStatusError, "redis": HealthStatusOK},
			err:  ErrUnavailable,
		},
		{
			name: "NilDependenciesSkipped",
			deps: func() HealthDependencies {
				pg := NewMockPinger(suite.T())
				pg.EXPECT().Ping(mock.Anything).Return(nil)
				var typedNil *typedNilPinger
				return HealthDependencies{"postgres": pg, "redis": nil, "s3": typedNil}
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

func (suite *HealthSuite) TestCheckCachesResult() {
	pg := NewMockPinger(suite.T())
	pg.EXPECT().Ping(mock.Anything).Return(nil).Once()
	uc := NewHealth(slogdiscard.NewDiscardLogger(), suite.cfg, HealthDependencies{"postgres": pg})

	for range 3 {
		got, err := uc.Check(context.Background())
		suite.NoError(err)
		suite.Equal(HealthReport{"postgres": HealthStatusOK}, got)
	}
}
