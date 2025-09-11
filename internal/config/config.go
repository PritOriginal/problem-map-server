package config

import (
	"flag"
	"os"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/spf13/viper"
)

type Config struct {
	Env    logger.Environment `mapstructure:"env"`
	Server ServerConfig       `mapstructure:"server"`
	GRPC   GRPCConfig         `mapstructure:"grpc"`
	DB     DatabaseConfig     `mapstructure:"db"`
	Redis  RedisConfig        `mapstructure:"redis"`
	Aws    AwsConfig          `mapstructure:"aws"`
}

type ServerConfig struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	Timeout struct {
		Server time.Duration `mapstructure:"server"`
		Write  time.Duration `mapstructure:"write"`
		Read   time.Duration `mapstructure:"read"`
		Idle   time.Duration `mapstructure:"idle"`
	} `mapstructure:"timeout"`
}

type GRPCConfig struct {
	Port    int           `mapstructure:"port"`
	Timeout time.Duration `mapstructure:"timeout"`
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

func MustLoad() *Config {
	configPath := fetchConfigPath()
	if configPath == "" {
		panic("config path is empty")
	}

	return MustLoadPath(configPath)
}

func MustLoadPath(configPath string) *Config {
	var cfg *Config

	viper.AddConfigPath(configPath)
	if err := viper.ReadInConfig(); err != nil {
		panic("failed read config file")
	}

	err := viper.Unmarshal(&cfg)
	if err != nil {
		panic("failed unmarshal config file")
	}

	return cfg
}

func fetchConfigPath() string {
	var res string

	flag.StringVar(&res, "config", "", "path to config file")
	flag.Parse()

	if res == "" {
		res = os.Getenv("CONFIG_PATH")
	}

	return res
}
