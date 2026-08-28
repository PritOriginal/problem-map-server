package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env          logger.Environment `yaml:"env" env:"ENV" env-default:"local"`
	REST         RESTConfig         `yaml:"rest"`
	GRPC         GRPCConfig         `yaml:"grpc"`
	PhotoStorage PhotoStorageType   `yaml:"photo-storage" env:"PHOTO_STORAGE" env-default:"local"`
	Auth         AuthConfig         `yaml:"auth"`
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

type AuthConfig struct {
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
	SSLMode  string `yaml:"sslmode" env:"POSTGRES_SSLMODE" env-default:"disable"`
	Pool     struct {
		MaxOpenConns    int           `yaml:"max_open_conns" env:"POSTGRES_MAX_OPEN_CONNS" env-default:"25"`
		MaxIdleConns    int           `yaml:"max_idle_conns" env:"POSTGRES_MAX_IDLE_CONNS" env-default:"5"`
		ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"POSTGRES_CONN_MAX_LIFETIME" env-default:"5m"`
	} `yaml:"pool"`
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
	cfg, err := LoadPath(configPath)
	if err != nil {
		panic(err.Error())
	}

	return cfg
}

// LoadPath reads the config file at configPath (env vars override file
// values) and validates every section.
func LoadPath(configPath string) (*Config, error) {
	cfg, err := ReadPath(configPath)
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ReadPath reads the config file at configPath without validating it.
// Callers that use only a part of the config (e.g. the migrator) can validate
// just that section.
func ReadPath(configPath string) (*Config, error) {
	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	return &cfg, nil
}

// DSN returns a postgres:// connection URL. Credentials are URL-escaped so
// that reserved characters in the password do not break the DSN.
func (d DatabaseConfig) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.Username, d.Password),
		Host:     net.JoinHostPort(d.Host, strconv.Itoa(d.Port)),
		Path:     "/" + d.Name,
		RawQuery: "sslmode=" + url.QueryEscape(d.SSLMode),
	}

	return u.String()
}

// MinJWTKeyLength is the minimum length (in bytes) of a JWT signing key.
// HMAC-SHA256 keys shorter than the hash output size are considered weak.
const MinJWTKeyLength = 32

// Validate checks that security-sensitive settings are present and sane.
func (c *Config) Validate() error {
	if err := errors.Join(c.Auth.Validate(), c.DB.Validate()); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}

// Validate checks that both JWT signing keys are present and strong enough.
func (a AuthConfig) Validate() error {
	return errors.Join(
		validateJWTKey("auth.jwt.access.key (JWT_ACCESS_TOKEN_KEY)", a.JWT.Access.Key),
		validateJWTKey("auth.jwt.refresh.key (JWT_REFRESH_TOKEN_KEY)", a.JWT.Refresh.Key),
	)
}

// Validate checks that the database password is set.
func (d DatabaseConfig) Validate() error {
	if d.Password == "" {
		return errors.New("db.password (POSTGRES_PASSWORD) must not be empty")
	}
	return nil
}

func validateJWTKey(name, key string) error {
	if key == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	if len(key) < MinJWTKeyLength {
		return fmt.Errorf("%s must be at least %d bytes long, got %d (generate one with `openssl rand -base64 32`)",
			name, MinJWTKeyLength, len(key))
	}
	return nil
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
