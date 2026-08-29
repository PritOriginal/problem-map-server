package config_test

import (
	"testing"

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
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{name: "Valid", mutate: func(*config.Config) {}},
		{name: "ShortAccessKey", mutate: func(c *config.Config) { c.Auth.JWT.Access.Key = "qwer" }, wantErr: "JWT_ACCESS_TOKEN_KEY"},
		{name: "EmptyRefreshKey", mutate: func(c *config.Config) { c.Auth.JWT.Refresh.Key = "" }, wantErr: "JWT_REFRESH_TOKEN_KEY"},
		{name: "EmptyDBPasswordAllowed", mutate: func(c *config.Config) { c.DB.Password = "" }},
		{name: "EmptyDBHost", mutate: func(c *config.Config) { c.DB.Host = "" }, wantErr: "POSTGRES_HOST"},
		{name: "EmptyDBUser", mutate: func(c *config.Config) { c.DB.Username = "" }, wantErr: "POSTGRES_USER"},
		{name: "EmptyDBName", mutate: func(c *config.Config) { c.DB.Name = "" }, wantErr: "POSTGRES_DB"},
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
