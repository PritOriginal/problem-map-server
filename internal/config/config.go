package config

import (
	"flag"
	"os"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env          logger.Environment `yaml:"env" env:"ENV" env-default:"local"`
	REST         RESTConfig         `yaml:"rest"`
	GRPC         GRPCConfig         `yaml:"grpc"`
	PhotoStorage PhotoStorageType   `yaml:"photo-storage" env:"PHOTO_STORAGE" env-default:"local"`
	Auth         AuthConfing        `yaml:"auth"`
	DB           DatabaseConfig     `yaml:"db"`
	Redis        RedisConfig        `yaml:"redis"`
	Aws          AwsConfig          `yaml:"aws"`
	Nats         NatsConfig         `yaml:"nats"`
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
		Write time.Duration `yaml:"write" env:"REST_TIMEOUT_WRITE"`
		Read  time.Duration `yaml:"read" env:"REST_TIMEOUT_READ"`
		Idle  time.Duration `yaml:"idle" env:"REST_TIMEOUT_IDLE"`
	} `yaml:"timeout"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	// TrustedProxies lists IPs/CIDRs of reverse proxies whose X-Forwarded-For
	// is trusted for the client IP (used by rate limiting). Empty means none.
	TrustedProxies []string `yaml:"trusted_proxies" env:"REST_TRUSTED_PROXIES"`
}

// RateLimitConfig limits requests per client IP for auth endpoints (signin, signup, refresh).
type RateLimitConfig struct {
	Requests int           `yaml:"requests" env:"REST_RATE_LIMIT_REQUESTS" env-default:"10"`
	Window   time.Duration `yaml:"window" env:"REST_RATE_LIMIT_WINDOW" env-default:"1m"`
}

type GRPCConfig struct {
	Port    int           `yaml:"port" env:"GRPC_PORT"`
	Timeout time.Duration `yaml:"timeout" env:"GRPC_TIMEOUT"`
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
