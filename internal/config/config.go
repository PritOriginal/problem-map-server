package config

import (
	"flag"
	"os"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env    logger.Environment `yaml:"env" env:"ENV" env-default:"local"`
	REST   RESTConfig         `yaml:"rest"`
	GRPC   GRPCConfig         `yaml:"grpc"`
	Health HealthConfig       `yaml:"health"`
	// ShutdownTimeout bounds graceful shutdown (draining requests and closing
	// clients); align it with the orchestrator's termination grace period.
	ShutdownTimeout time.Duration    `yaml:"shutdown-timeout" env:"SHUTDOWN_TIMEOUT" env-default:"10s"`
	PhotoStorage    PhotoStorageType `yaml:"photo-storage" env:"PHOTO_STORAGE" env-default:"local"`
	Auth            AuthConfing      `yaml:"auth"`
	DB              DatabaseConfig   `yaml:"db"`
	Redis           RedisConfig      `yaml:"redis"`
	Aws             AwsConfig        `yaml:"aws"`
	Nats            NatsConfig       `yaml:"nats"`
}

type PhotoStorageType string

const (
	Local PhotoStorageType = "local"
	S3    PhotoStorageType = "s3"
)

type RESTConfig struct {
	Host    string `yaml:"host" env:"REST_HOST"`
	Port    int    `yaml:"port" env:"REST_PORT"`
	Timeout struct {
		Write time.Duration `yaml:"write" env:"REST_TIMEOUT_WRITE" env-default:"10s"`
		Read  time.Duration `yaml:"read" env:"REST_TIMEOUT_READ" env-default:"15s"`
		Idle  time.Duration `yaml:"idle" env:"REST_TIMEOUT_IDLE" env-default:"60s"`
	} `yaml:"timeout"`
}

// HealthConfig tunes the readiness probe.
type HealthConfig struct {
	// Timeout bounds a single dependency ping.
	Timeout time.Duration `yaml:"timeout" env:"HEALTH_TIMEOUT" env-default:"3s"`
	// CacheTTL is how long a readiness result is reused before re-pinging.
	CacheTTL time.Duration `yaml:"cache-ttl" env:"HEALTH_CACHE_TTL" env-default:"2s"`
}

type GRPCConfig struct {
	Port int `yaml:"port" env:"GRPC_PORT"`
	// Timeout is the per-RPC deadline used by clients (functional tests).
	Timeout time.Duration `yaml:"timeout" env:"GRPC_TIMEOUT"`
	// ConnectionTimeout bounds connection establishment (handshake) on the server.
	ConnectionTimeout time.Duration `yaml:"connection-timeout" env:"GRPC_CONNECTION_TIMEOUT" env-default:"120s"`
	// MetricsPort is the HTTP port serving Prometheus metrics for the gRPC
	// server; 0 disables the metrics endpoint.
	MetricsPort int `yaml:"metrics-port" env:"GRPC_METRICS_PORT" env-default:"0"`
}

type AuthConfing struct {
	JWT struct {
		Access struct {
			Key       string        `yaml:"key" env:"JWT_ACCESS_TOKEN_KEY"`
			ExpiredIn time.Duration `yaml:"expired_in" env:"JWT_ACCESS_TOKEN_EXPIRED_IN"`
		} `yaml:"access"`
		Refresh struct {
			Key       string        `yaml:"key" env:"JWT_REFRESH_TOKEN_KEY"`
			ExpiredIn time.Duration `yaml:"expired_in" env:"JWT_REFRESH_TOKEN_EXPIRED_IN"`
		} `yaml:"refresh"`
	} `yaml:"jwt"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host" env:"POSTGRES_HOST"`
	Port     int    `yaml:"port" env:"POSTGRES_PORT"`
	Username string `yaml:"username" env:"POSTGRES_USER"`
	Password string `yaml:"password" env:"POSTGRES_PASSWORD"`
	Name     string `yaml:"name" env:"POSTGRES_DB"`
}

type RedisConfig struct {
	Host     string `yaml:"host" env:"REDIS_HOST"`
	Port     int    `yaml:"port" env:"REDIS_PORT"`
	Password string `yaml:"password" env:"REDIS_PASSWORD"`
}

type AwsConfig struct {
	Key       string `yaml:"key" env:"AWS_KEY"`
	SecretKey string `yaml:"secret_key" env:"AWS_SECRET_KEY"`
	EndPoint  string `yaml:"endpoint" env:"AWS_ENDPOINT"`
}

type NatsConfig struct {
	URL  string `yaml:"url" env:"NATS_URL"`
	Name string `yaml:"name" env:"NATS_NAME"`
}

func MustLoad() *Config {
	configPath := fetchConfigPath()
	if configPath == "" {
		panic("config path is empty")
	}

	return MustLoadPath(configPath)
}

func MustLoadPath(configPath string) *Config {
	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		panic("cannot read config: " + err.Error())
	}

	return &cfg
}

// fetchConfigPath fetches config path from command line flag or environment variable.
// Priority: flag > env > default.
// Default value is empty string.
func fetchConfigPath() string {
	var res string

	flag.StringVar(&res, "config", "", "path to config file")
	flag.Parse()

	if res == "" {
		res = os.Getenv("CONFIG_PATH")
	}

	return res
}
