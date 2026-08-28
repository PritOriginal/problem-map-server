package config_test

import (
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.DatabaseConfig
		want string
	}{
		{
			name: "plain",
			cfg:  config.DatabaseConfig{Host: "localhost", Port: 5432, Username: "postgres", Password: "postgres", Name: "problem_map", SSLMode: "require"},
			want: "postgres://postgres:postgres@localhost:5432/problem_map?sslmode=require",
		},
		{
			name: "escapes reserved characters and defaults sslmode",
			cfg:  config.DatabaseConfig{Host: "db", Port: 5432, Username: "us er", Password: "p@ss/w#rd", Name: "pm"},
			want: "postgres://us%20er:p%40ss%2Fw%23rd@db:5432/pm?sslmode=disable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.DSN())
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	longKey := "0123456789abcdef0123456789abcdef"

	valid := func() config.Config {
		var c config.Config
		c.Auth.JWT.Access.Key = longKey
		c.Auth.JWT.Refresh.Key = longKey
		c.DB.Password = "secret"
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{name: "valid", mutate: func(*config.Config) {}},
		{name: "short access key", mutate: func(c *config.Config) { c.Auth.JWT.Access.Key = "qwer" }, wantErr: "JWT_ACCESS_TOKEN_KEY"},
		{name: "empty refresh key", mutate: func(c *config.Config) { c.Auth.JWT.Refresh.Key = "" }, wantErr: "JWT_REFRESH_TOKEN_KEY"},
		{name: "empty db password", mutate: func(c *config.Config) { c.DB.Password = "" }, wantErr: "POSTGRES_PASSWORD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
