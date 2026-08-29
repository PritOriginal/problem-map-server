package config_test

import (
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (suite *ConfigSuite) TestDatabaseConfigDSN() {
	tests := []struct {
		name string
		cfg  config.DatabaseConfig
		want string
	}{
		{
			name: "Plain",
			cfg:  config.DatabaseConfig{Host: "localhost", Port: 5432, Username: "postgres", Password: "postgres", Name: "problem_map", SSLMode: "require"},
			want: "postgres://postgres:postgres@localhost:5432/problem_map?sslmode=require",
		},
		{
			name: "EscapesReservedCharacters",
			cfg:  config.DatabaseConfig{Host: "db", Port: 5432, Username: "us er", Password: "p@ss/w#rd", Name: "pm", SSLMode: "disable"},
			want: "postgres://us%20er:p%40ss%2Fw%23rd@db:5432/pm?sslmode=disable",
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.want, tt.cfg.DSN())
		})
	}
}

func (suite *ConfigSuite) TestValidate() {
	longKey := "0123456789abcdef0123456789abcdef"

	valid := func() config.Config {
		var c config.Config
		c.Auth.JWT.Access.Key = longKey
		c.Auth.JWT.Refresh.Key = longKey
		c.DB.Host = "localhost"
		c.DB.Username = "postgres"
		c.DB.Name = "problem_map"
		c.DB.Password = "secret"
		c.Tasker = config.TaskerConfig{
			Interval: time.Minute, TaskTTL: time.Hour, MaxTasksPerUser: 1, RequiredChecks: 1,
			TargetProbability: 0.5, MaxRadiusMeters: 100,
		}
		c.Marks = config.MarksConfig{DedupRadiusM: 50}
		c.Rating = config.RatingConfig{CheckCorrect: 2, CheckWrong: -1, MarkConfirmed: 3, MarkRefuted: -2, TaskCompleted: 1, MaxChecksPerDay: 50}
		c.Export.MaxRows = 50_000
		c.Export.RateLimit.Requests = 2
		c.Export.RateLimit.Window = time.Minute
		c.Webhooks = config.WebhooksConfig{Timeout: 10 * time.Second, RetryInterval: 30 * time.Second, RetryBatch: 100}
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{name: "Valid", mutate: func(*config.Config) {}},
		{name: "zero checks per day", mutate: func(c *config.Config) { c.Rating.MaxChecksPerDay = 0 }, wantErr: "RATING_MAX_CHECKS_PER_DAY"},
		{name: "ShortAccessKey", mutate: func(c *config.Config) { c.Auth.JWT.Access.Key = "qwer" }, wantErr: "JWT_ACCESS_TOKEN_KEY"},
		{name: "EmptyRefreshKey", mutate: func(c *config.Config) { c.Auth.JWT.Refresh.Key = "" }, wantErr: "JWT_REFRESH_TOKEN_KEY"},
		{name: "EmptyDBPasswordAllowed", mutate: func(c *config.Config) { c.DB.Password = "" }},
		{name: "EmptyDBHost", mutate: func(c *config.Config) { c.DB.Host = "" }, wantErr: "POSTGRES_HOST"},
		{name: "EmptyDBUser", mutate: func(c *config.Config) { c.DB.Username = "" }, wantErr: "POSTGRES_USER"},
		{name: "EmptyDBName", mutate: func(c *config.Config) { c.DB.Name = "" }, wantErr: "POSTGRES_DB"},
		{name: "ZeroDedupRadius", mutate: func(c *config.Config) { c.Marks.DedupRadiusM = 0 }, wantErr: "MARKS_DEDUP_RADIUS_M"},
		{name: "valid", mutate: func(*config.Config) {}},
		{name: "short access key", mutate: func(c *config.Config) { c.Auth.JWT.Access.Key = "qwer" }, wantErr: "JWT_ACCESS_TOKEN_KEY"},
		{name: "empty refresh key", mutate: func(c *config.Config) { c.Auth.JWT.Refresh.Key = "" }, wantErr: "JWT_REFRESH_TOKEN_KEY"},
		{name: "zero tasker interval", mutate: func(c *config.Config) { c.Tasker.Interval = 0 }, wantErr: "TASKER_INTERVAL"},
		{name: "target probability above one", mutate: func(c *config.Config) { c.Tasker.TargetProbability = 1.5 }, wantErr: "TASKER_TARGET_PROBABILITY"},
		{name: "negative factor", mutate: func(c *config.Config) { c.Tasker.LoadDelta = -1 }, wantErr: "load-delta"},
		{name: "zero export rows", mutate: func(c *config.Config) { c.Export.MaxRows = 0 }, wantErr: "EXPORT_MAX_ROWS"},
		{name: "negative export rate limit", mutate: func(c *config.Config) { c.Export.RateLimit.Requests = -1 }, wantErr: "export.rate-limit"},
		{name: "zero webhook timeout", mutate: func(c *config.Config) { c.Webhooks.Timeout = 0 }, wantErr: "WEBHOOKS_TIMEOUT"},
		{name: "zero webhook retry interval", mutate: func(c *config.Config) { c.Webhooks.RetryInterval = 0 }, wantErr: "WEBHOOKS_RETRY_INTERVAL"},
		{name: "zero webhook retry batch", mutate: func(c *config.Config) { c.Webhooks.RetryBatch = 0 }, wantErr: "WEBHOOKS_RETRY_BATCH"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			c := valid()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				suite.NoError(err)
				return
			}
			suite.ErrorContains(err, tt.wantErr)
		})
	}
}
