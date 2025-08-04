package configs

import (
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/spf13/viper"
)

type Config struct {
	Env    logger.Environment `mapstructure:"env"`
	Server ServerConfig       `mapstructure:"server"`
	DB     DatabaseConfig     `mapstructure:"db"`
	Redis  RedisConfig        `mapstructure:"redis"`
	Aws    AwsConfig          `mapstructure:"aws"`
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

type RedisConfig struct {
	Host     string
	Port     int
	Password string
}

type AwsConfig struct {
	Key       string
	SecretKey string
	EndPoint  string
}

func Init() (*Config, error) {
	var cfg *Config

	viper.AddConfigPath("./configs")
	if err := viper.ReadInConfig(); err != nil {
		return cfg, err
	}

	err := viper.Unmarshal(&cfg)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}
