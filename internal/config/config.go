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
	Env    logger.Environment `yaml:"env" env:"ENV" env-default:"local"`
	REST   RESTConfig         `yaml:"rest"`
	GRPC   GRPCConfig         `yaml:"grpc"`
	Health HealthConfig       `yaml:"health"`
	// ShutdownTimeout bounds graceful shutdown (draining requests and closing
	// clients); align it with the orchestrator's termination grace period.
	ShutdownTimeout time.Duration    `yaml:"shutdown-timeout" env:"SHUTDOWN_TIMEOUT" env-default:"10s"`
	PhotoStorage    PhotoStorageType `yaml:"photo-storage" env:"PHOTO_STORAGE" env-default:"local"`
	Auth            AuthConfig       `yaml:"auth"`
	DB              DatabaseConfig   `yaml:"db"`
	Redis           RedisConfig      `yaml:"redis"`
	Aws             AwsConfig        `yaml:"aws"`
	Nats            NatsConfig       `yaml:"nats"`
	Tasker          TaskerConfig     `yaml:"tasker"`
	Marks           MarksConfig      `yaml:"marks"`
	Rating          RatingConfig     `yaml:"rating"`
}

// MarksConfig tunes mark creation.
type MarksConfig struct {
	// DedupRadiusM is the radius (meters) within which an active mark of the
	// same type is treated as a duplicate on POST /marks.
	DedupRadiusM float64 `yaml:"dedup-radius-m" env:"MARKS_DEDUP_RADIUS_M" env-default:"50"`
}

// Validate checks that the dedup radius is sane.
func (m MarksConfig) Validate() error {
	if m.DedupRadiusM <= 0 || m.DedupRadiusM > 50_000 {
		return errors.New("marks.dedup-radius-m (MARKS_DEDUP_RADIUS_M) must be in (0, 50000]")
	}
	return nil
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

// TaskerConfig tunes the task assignment job (cmd/tasker).
//
// Probability that a user verifies a mark:
//
//	p = (rating(r) + distance(d)) * load(l) * fatigue(o)
//	rating(r)   = 0.2 / (1 + 100*exp(-r/2))
//	distance(d) = 0.5 * exp(-DistanceLambda * d_km)
//	load(l)     = 1 / (1 + LoadDelta * (l + 1))      l — issued tasks of the user
//	fatigue(o)  = 1 / (1 + FatigueBeta * o)          o — overdue tasks of the user
//
// A mark is considered covered when the probability that at least
// RequiredChecks of its assignees verify it reaches TargetProbability.
type TaskerConfig struct {
	// Interval between scheduled runs (scheduled mode of cmd/tasker).
	Interval time.Duration `yaml:"interval" env:"TASKER_INTERVAL" env-default:"15m"`
	// TaskTTL is how long an issued task stays valid; after that it is
	// marked overdue (tasks.due_at = issued_at + TaskTTL).
	TaskTTL time.Duration `yaml:"task-ttl" env:"TASKER_TASK_TTL" env-default:"72h"`
	// MaxTasksPerUser caps simultaneously issued tasks per user.
	MaxTasksPerUser int `yaml:"max-tasks-per-user" env:"TASKER_MAX_TASKS_PER_USER" env-default:"3"`
	// RequiredChecks is how many independent verifications a mark needs.
	RequiredChecks int `yaml:"required-checks" env:"TASKER_REQUIRED_CHECKS" env-default:"2"`
	// TargetProbability at which a mark stops receiving new assignees.
	TargetProbability float64 `yaml:"target-probability" env:"TASKER_TARGET_PROBABILITY" env-default:"0.8"`
	// MaxRadiusMeters limits the distance between a mark and a user's home.
	MaxRadiusMeters int `yaml:"max-radius-meters" env:"TASKER_MAX_RADIUS_METERS" env-default:"5000"`
	// DistanceLambda is the decay of the distance factor per kilometre.
	DistanceLambda float64 `yaml:"distance-lambda" env:"TASKER_DISTANCE_LAMBDA" env-default:"0.05"`
	// LoadDelta is the penalty per issued task.
	LoadDelta float64 `yaml:"load-delta" env:"TASKER_LOAD_DELTA" env-default:"0.3"`
	// FatigueBeta is the penalty per overdue task.
	FatigueBeta float64 `yaml:"fatigue-beta" env:"TASKER_FATIGUE_BETA" env-default:"0.2"`
}

// Validate checks that the tasker parameters are sane.
func (t TaskerConfig) Validate() error {
	var errs []error
	if t.Interval <= 0 {
		errs = append(errs, errors.New("tasker.interval (TASKER_INTERVAL) must be positive"))
	}
	if t.TaskTTL <= 0 {
		errs = append(errs, errors.New("tasker.task-ttl (TASKER_TASK_TTL) must be positive"))
	}
	if t.MaxTasksPerUser <= 0 {
		errs = append(errs, errors.New("tasker.max-tasks-per-user (TASKER_MAX_TASKS_PER_USER) must be positive"))
	}
	if t.RequiredChecks <= 0 {
		errs = append(errs, errors.New("tasker.required-checks (TASKER_REQUIRED_CHECKS) must be positive"))
	}
	if t.TargetProbability < 0 || t.TargetProbability > 1 {
		errs = append(errs, errors.New("tasker.target-probability (TASKER_TARGET_PROBABILITY) must be in [0, 1]"))
	}
	if t.MaxRadiusMeters <= 0 {
		errs = append(errs, errors.New("tasker.max-radius-meters (TASKER_MAX_RADIUS_METERS) must be positive"))
	}
	if t.DistanceLambda < 0 || t.LoadDelta < 0 || t.FatigueBeta < 0 {
		errs = append(errs, errors.New("tasker.distance-lambda, load-delta and fatigue-beta must not be negative"))
	}
	return errors.Join(errs...)
}

// RatingConfig holds the rating deltas awarded when a mark's voting stage
// resolves (see usecase.Updater) and the anti-fraud limits of AddCheck.
type RatingConfig struct {
	// CheckCorrect is awarded to a checker whose vote matched the outcome.
	CheckCorrect int `yaml:"check-correct" env:"RATING_CHECK_CORRECT" env-default:"2"`
	// CheckWrong is awarded (usually negative) to a checker whose vote did not.
	CheckWrong int `yaml:"check-wrong" env:"RATING_CHECK_WRONG" env-default:"-1"`
	// MarkConfirmed is awarded to the author when a mark gets confirmed.
	MarkConfirmed int `yaml:"mark-confirmed" env:"RATING_MARK_CONFIRMED" env-default:"3"`
	// MarkRefuted is awarded to the author when a mark gets refuted.
	MarkRefuted int `yaml:"mark-refuted" env:"RATING_MARK_REFUTED" env-default:"-2"`
	// TaskCompleted is awarded when a check closes an issued task.
	TaskCompleted int `yaml:"task-completed" env:"RATING_TASK_COMPLETED" env-default:"1"`
	// MaxChecksPerDay caps the checks a user may submit in a rolling 24 hours.
	MaxChecksPerDay int `yaml:"max-checks-per-day" env:"RATING_MAX_CHECKS_PER_DAY" env-default:"50"`
}

// Validate checks that the anti-fraud limit is sane.
func (r RatingConfig) Validate() error {
	if r.MaxChecksPerDay <= 0 {
		return errors.New("rating.max-checks-per-day (RATING_MAX_CHECKS_PER_DAY) must be positive")
	}
	return nil
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
	if err := errors.Join(c.Auth.Validate(), c.DB.Validate(), c.Tasker.Validate(), c.Marks.Validate(), c.Rating.Validate()); err != nil {
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

// Validate checks that the settings needed to reach the database are set.
// The password is deliberately not required: trust/peer authentication and
// managed connection proxies work without one.
func (d DatabaseConfig) Validate() error {
	var errs []error
	if d.Host == "" {
		errs = append(errs, errors.New("db.host (POSTGRES_HOST) must not be empty"))
	}
	if d.Username == "" {
		errs = append(errs, errors.New("db.username (POSTGRES_USER) must not be empty"))
	}
	if d.Name == "" {
		errs = append(errs, errors.New("db.name (POSTGRES_DB) must not be empty"))
	}
	return errors.Join(errs...)
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

	return ResolveConfigPath(res)
}

// ResolveConfigPath applies the shared priority rule for the config path:
// the explicit (flag) value wins, otherwise CONFIG_PATH is used.
func ResolveConfigPath(flagValue string) string {
	if flagValue == "" {
		return os.Getenv("CONFIG_PATH")
	}
	return flagValue
}
