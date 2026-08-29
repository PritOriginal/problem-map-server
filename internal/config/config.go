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
	Notifier        NotifierConfig   `yaml:"notifier"`
	Tasker          TaskerConfig     `yaml:"tasker"`
	Marks           MarksConfig      `yaml:"marks"`
	Comments        CommentsConfig   `yaml:"comments"`
	Rating          RatingConfig     `yaml:"rating"`
	Push            PushConfig       `yaml:"push"`
	Export          ExportConfig     `yaml:"export"`
	Webhooks        WebhooksConfig   `yaml:"webhooks"`
}

// NotifierConfig tunes the notification worker (cmd/notifier).
type NotifierConfig struct {
	// MetricsPort is the HTTP port serving Prometheus metrics of the worker
	// (push_sent_total etc.); 0 disables the endpoint.
	MetricsPort int `yaml:"metrics-port" env:"NOTIFIER_METRICS_PORT" env-default:"0"`
}

// PushConfig configures push delivery in cmd/notifier. Without FCM
// credentials the worker only logs what would be sent.
type PushConfig struct {
	// SendTimeout bounds the delivery of one notification to all devices of
	// its addressee (including retries).
	SendTimeout time.Duration `yaml:"send-timeout" env:"PUSH_SEND_TIMEOUT" env-default:"15s"`
	FCM         FCMConfig     `yaml:"fcm"`
	APNs        APNsConfig    `yaml:"apns"`
}

// FCMConfig configures Firebase Cloud Messaging (HTTP v1 API), used for
// android and web devices. Credentials are a Google service account key in
// JSON: either a path to the file or the JSON itself (handy for containers).
type FCMConfig struct {
	// ProjectID is the Firebase project id; empty means "take project_id
	// from the credentials".
	ProjectID       string `yaml:"project-id" env:"FCM_PROJECT_ID"`
	CredentialsFile string `yaml:"credentials-file" env:"FCM_CREDENTIALS_FILE"`
	CredentialsJSON string `yaml:"credentials-json" env:"FCM_CREDENTIALS_JSON"`
	// Timeout bounds a single HTTP request to FCM.
	Timeout time.Duration `yaml:"timeout" env:"FCM_TIMEOUT" env-default:"5s"`
	// MaxRetries is how many times a request is repeated on 5xx/429 (0-3).
	MaxRetries int `yaml:"max-retries" env:"FCM_MAX_RETRIES" env-default:"3"`
	// Concurrency caps simultaneous requests to FCM.
	Concurrency int `yaml:"concurrency" env:"FCM_CONCURRENCY" env-default:"8"`
}

// Enabled reports whether FCM credentials are configured.
func (f FCMConfig) Enabled() bool {
	return f.CredentialsFile != "" || f.CredentialsJSON != ""
}

// Validate checks the FCM settings; an unconfigured FCM is valid.
func (f FCMConfig) Validate() error {
	var errs []error
	if f.CredentialsFile != "" && f.CredentialsJSON != "" {
		errs = append(errs, errors.New("push.fcm: set either credentials-file (FCM_CREDENTIALS_FILE) or credentials-json (FCM_CREDENTIALS_JSON), not both"))
	}
	if f.Timeout <= 0 {
		errs = append(errs, errors.New("push.fcm.timeout (FCM_TIMEOUT) must be positive"))
	}
	if f.MaxRetries < 0 || f.MaxRetries > 3 {
		errs = append(errs, errors.New("push.fcm.max-retries (FCM_MAX_RETRIES) must be in [0, 3]"))
	}
	if f.Concurrency <= 0 {
		errs = append(errs, errors.New("push.fcm.concurrency (FCM_CONCURRENCY) must be positive"))
	}
	return errors.Join(errs...)
}

// APNsConfig configures Apple Push Notification service (token-based auth)
// for ios devices. The APNs sender is not implemented yet (see
// internal/push/apns); the settings are reserved so that deployments can be
// prepared ahead of time.
type APNsConfig struct {
	// KeyFile is the path to the .p8 signing key from the Apple developer
	// account; empty disables APNs.
	KeyFile  string `yaml:"key-file" env:"APNS_KEY_FILE"`
	KeyID    string `yaml:"key-id" env:"APNS_KEY_ID"`
	TeamID   string `yaml:"team-id" env:"APNS_TEAM_ID"`
	BundleID string `yaml:"bundle-id" env:"APNS_BUNDLE_ID"`
	// Sandbox selects the development APNs environment.
	Sandbox bool `yaml:"sandbox" env:"APNS_SANDBOX" env-default:"false"`
}

// Enabled reports whether APNs credentials are configured.
func (a APNsConfig) Enabled() bool {
	return a.KeyFile != ""
}

// Validate checks that the push settings are sane.
func (p PushConfig) Validate() error {
	var errs []error
	if p.SendTimeout <= 0 {
		errs = append(errs, errors.New("push.send-timeout (PUSH_SEND_TIMEOUT) must be positive"))
	}
	errs = append(errs, p.FCM.Validate())
	return errors.Join(errs...)
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

// CommentsConfig tunes the comments on marks.
type CommentsConfig struct {
	// EditWindow is how long after creation the author may edit a comment.
	EditWindow time.Duration `yaml:"edit-window" env:"COMMENTS_EDIT_WINDOW" env-default:"15m"`
	// MaxPerDay caps the comments a user may post in a rolling 24 hours.
	MaxPerDay int `yaml:"max-per-day" env:"COMMENTS_MAX_PER_DAY" env-default:"100"`
}

// Validate checks that the comment limits are sane.
func (c CommentsConfig) Validate() error {
	var errs []error
	if c.EditWindow <= 0 {
		errs = append(errs, errors.New("comments.edit-window (COMMENTS_EDIT_WINDOW) must be positive"))
	}
	if c.MaxPerDay <= 0 {
		errs = append(errs, errors.New("comments.max-per-day (COMMENTS_MAX_PER_DAY) must be positive"))
	}
	return errors.Join(errs...)
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

// NatsDelivery selects how domain events travel through the broker.
type NatsDelivery string

const (
	// NatsDeliveryJetStream persists events in a JetStream stream
	// (at-least-once, deduplicated by event_id, dead-letter queue). The
	// client falls back to core NATS with a warning when the server has
	// JetStream disabled.
	NatsDeliveryJetStream NatsDelivery = "jetstream"
	// NatsDeliveryCore publishes with core NATS (at-most-once): an event
	// published while no consumer is connected is lost.
	NatsDeliveryCore NatsDelivery = "core"
)

// NatsConfig configures the domain-event broker. An empty URL disables
// publishing (events.NoopPublisher) for the servers and is a startup error
// for cmd/notifier, which cannot work without a broker.
type NatsConfig struct {
	URL  string `yaml:"url" env:"NATS_URL" env-default:""`
	Name string `yaml:"name" env:"NATS_NAME" env-default:"problem-map"`
	// Delivery is "jetstream" (default, also for an empty value) or
	// "core". It is a string rather
	// than a bool because cleanenv replaces a false read from YAML with the
	// env-default, so a `jetstream: false` flag could never be switched off
	// from the file.
	Delivery NatsDelivery `yaml:"delivery" env:"NATS_DELIVERY" env-default:"jetstream"`
}

// JetStream reports whether events go through JetStream.
func (n NatsConfig) JetStream() bool { return n.Delivery != NatsDeliveryCore }

// Validate checks that the broker URL is set (required by cmd/notifier) and
// the delivery mode is known.
func (n NatsConfig) Validate() error {
	var errs []error
	if n.URL == "" {
		errs = append(errs, errors.New("nats.url (NATS_URL) must not be empty"))
	}
	if err := n.ValidateDelivery(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// ValidateDelivery checks only the delivery mode; the servers call it
// because for them an empty URL is allowed.
func (n NatsConfig) ValidateDelivery() error {
	switch n.Delivery {
	case "", NatsDeliveryJetStream, NatsDeliveryCore:
		return nil
	default:
		return fmt.Errorf("nats.delivery (NATS_DELIVERY) must be %q or %q, got %q",
			NatsDeliveryJetStream, NatsDeliveryCore, n.Delivery)
	}
}

// ExportConfig tunes GET /marks/export.
type ExportConfig struct {
	// MaxRows caps the number of marks one export may contain; a request
	// matching more rows is rejected with 400 so the client narrows the
	// filters.
	MaxRows int `yaml:"max-rows" env:"EXPORT_MAX_ROWS" env-default:"50000"`
	// RateLimit limits export requests per client IP (default 2 per minute).
	RateLimit struct {
		Requests int           `yaml:"requests" env:"EXPORT_RATE_LIMIT_REQUESTS" env-default:"2"`
		Window   time.Duration `yaml:"window" env:"EXPORT_RATE_LIMIT_WINDOW" env-default:"1m"`
	} `yaml:"rate-limit"`
}

// Validate checks that the export limits are sane.
func (e ExportConfig) Validate() error {
	var errs []error
	if e.MaxRows <= 0 {
		errs = append(errs, errors.New("export.max-rows (EXPORT_MAX_ROWS) must be positive"))
	}
	if e.RateLimit.Requests < 0 || e.RateLimit.Window < 0 {
		errs = append(errs, errors.New("export.rate-limit.requests and window must not be negative"))
	}
	return errors.Join(errs...)
}

// WebhooksConfig tunes outgoing webhook delivery (cmd/notifier and the
// test endpoint of the REST server).
type WebhooksConfig struct {
	// Timeout bounds one HTTP delivery attempt.
	Timeout time.Duration `yaml:"timeout" env:"WEBHOOKS_TIMEOUT" env-default:"10s"`
	// RetryInterval is how often the notifier looks for deliveries due for
	// another attempt.
	RetryInterval time.Duration `yaml:"retry-interval" env:"WEBHOOKS_RETRY_INTERVAL" env-default:"30s"`
	// RetryBatch caps the deliveries retried per tick.
	RetryBatch int `yaml:"retry-batch" env:"WEBHOOKS_RETRY_BATCH" env-default:"100"`
	// AllowPrivateURLs disables the SSRF guard (loopback/private/link-local
	// targets are rejected by default). Only for local development.
	AllowPrivateURLs bool `yaml:"allow-private-urls" env:"WEBHOOKS_ALLOW_PRIVATE_URLS" env-default:"false"`
}

// Validate checks that the delivery parameters are sane.
func (w WebhooksConfig) Validate() error {
	var errs []error
	if w.Timeout <= 0 {
		errs = append(errs, errors.New("webhooks.timeout (WEBHOOKS_TIMEOUT) must be positive"))
	}
	if w.RetryInterval <= 0 {
		errs = append(errs, errors.New("webhooks.retry-interval (WEBHOOKS_RETRY_INTERVAL) must be positive"))
	}
	if w.RetryBatch <= 0 {
		errs = append(errs, errors.New("webhooks.retry-batch (WEBHOOKS_RETRY_BATCH) must be positive"))
	}
	return errors.Join(errs...)
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
// that reserved characters in the password do not break the DSN. The
// session time zone is pinned to UTC (lib/pq forwards `timezone` as a
// startup parameter) so that TIMESTAMPTZ values are rendered and
// date_trunc'ed identically regardless of the server's default.
func (d DatabaseConfig) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.Username, d.Password),
		Host:     net.JoinHostPort(d.Host, strconv.Itoa(d.Port)),
		Path:     "/" + d.Name,
		RawQuery: "sslmode=" + url.QueryEscape(d.SSLMode) + "&timezone=UTC",
	}

	return u.String()
}

// MinJWTKeyLength is the minimum length (in bytes) of a JWT signing key.
// HMAC-SHA256 keys shorter than the hash output size are considered weak.
const MinJWTKeyLength = 32

// Validate checks that security-sensitive settings are present and sane.
func (c *Config) Validate() error {
	if err := errors.Join(c.Auth.Validate(), c.DB.Validate(), c.Tasker.Validate(), c.Marks.Validate(), c.Comments.Validate(), c.Rating.Validate(), c.Push.Validate(), c.Nats.ValidateDelivery(), c.Export.Validate(), c.Webhooks.Validate()); err != nil {
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
