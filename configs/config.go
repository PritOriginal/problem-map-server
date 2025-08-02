package configs

import "time"

type Config struct {
	Env    string         `mapstructure:"env"`
	Server ServerConfig   `mapstructure:"server"`
	DB     DatabaseConfig `mapstructure:"DB"`
}

type ServerConfig struct {
	Host    string `mapstructure:"host"`
	Port    string `mapstructure:"port"`
	Timeout struct {
		Server time.Duration `mapstructure:"server"`
		Write  time.Duration `mapstructure:"write"`
		Read   time.Duration `mapstructure:"read"`
		Idle   time.Duration `mapstructure:"idle"`
	} `mapstructure:"timeout"`
}

type DatabaseConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Name     string
}
