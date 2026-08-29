package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/guregu/null/v6"
)

// RuntimeSettingsKey is the settings row holding RuntimeSettings.
const RuntimeSettingsKey = "runtime"

// SettingsCacheTTL is how long a Settings instance reuses the value read
// from the database. A PUT invalidates the cache of the instance that served
// it at once; other instances (the gRPC server, cmd/tasker, replicas) pick
// the change up within this TTL. The TTL is short enough for an admin change
// to be effectively immediate and long enough to keep the hot paths
// (AddCheck, AddMark) off the settings table.
const SettingsCacheTTL = 30 * time.Second

// Limits of the editable settings; PUT /admin/settings rejects values
// outside them with ErrInvalidArgument.
const (
	MaxVoteThreshold   = 100
	MaxDedupRadiusM    = 50_000
	MaxChecksPerDayCap = 10_000
	MaxRatingDelta     = 1_000
	MaxTasksPerUserCap = 100
	MaxRequiredChecks  = 100
	MaxTaskRadiusM     = 100_000
	MaxTaskTTL         = Duration(24 * 365 * time.Hour)
)

// Duration is a time.Duration that travels through JSON as a Go duration
// string ("24h", "1h30m"); whole hours are written as "<n>h".
type Duration time.Duration

func (d Duration) String() string {
	dur := time.Duration(d)
	if dur%time.Hour == 0 {
		return fmt.Sprintf("%dh", dur/time.Hour)
	}
	return dur.String()
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var raw string
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("duration must be a string like \"24h\": %w", err)
	}
	dur, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid duration %q", raw)
	}
	*d = Duration(dur)
	return nil
}

// RuntimeSettings are the parameters administrators may change at runtime
// (GET/PUT /admin/settings). They override the corresponding config values
// (config.MarksConfig, RatingConfig, TaskerConfig), which only provide the
// defaults until the first PUT.
type RuntimeSettings struct {
	// VoteThreshold is the vote score (confirming minus refuting checks) at
	// which a voting stage resolves (see Updater.Update).
	VoteThreshold int `json:"vote_threshold"`
	// DedupRadiusM is the radius (meters) within which an active mark of the
	// same type is treated as a duplicate on POST /marks.
	DedupRadiusM int `json:"dedup_radius_m"`
	// MaxChecksPerDay caps the checks a user may submit in a rolling 24 hours.
	MaxChecksPerDay int            `json:"max_checks_per_day"`
	Rating          RatingSettings `json:"rating"`
	Tasker          TaskerSettings `json:"tasker"`
}

// RatingSettings are the rating deltas awarded when a voting stage resolves.
type RatingSettings struct {
	CheckCorrect  int `json:"check_correct"`
	CheckWrong    int `json:"check_wrong"`
	MarkConfirmed int `json:"mark_confirmed"`
	MarkRefuted   int `json:"mark_refuted"`
	TaskCompleted int `json:"task_completed"`
}

// TaskerSettings are the limits of the task assignment job.
type TaskerSettings struct {
	MaxTasksPerUser   int     `json:"max_tasks_per_user"`
	RequiredChecks    int     `json:"required_checks"`
	TargetProbability float64 `json:"target_probability"`
	MaxRadiusMeters   int     `json:"max_radius_meters"`
	// TaskTTL is how long an issued task stays valid (a duration string
	// such as "72h").
	TaskTTL Duration `json:"task_ttl" swaggertype:"string" example:"72h"`
}

// DefaultRuntimeSettings are the built-in defaults; they match the
// env-defaults of the config sections.
func DefaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		VoteThreshold:   3,
		DedupRadiusM:    int(models.DefaultDedupRadiusM),
		MaxChecksPerDay: 50,
		Rating: RatingSettings{
			CheckCorrect:  2,
			CheckWrong:    -1,
			MarkConfirmed: 3,
			MarkRefuted:   -2,
			TaskCompleted: 1,
		},
		Tasker: TaskerSettings{
			MaxTasksPerUser:   3,
			RequiredChecks:    2,
			TargetProbability: 0.8,
			MaxRadiusMeters:   5000,
			TaskTTL:           Duration(72 * time.Hour),
		},
	}
}

// RuntimeSettingsFromConfig builds the defaults from the loaded config.
func RuntimeSettingsFromConfig(cfg *config.Config) RuntimeSettings {
	s := DefaultRuntimeSettings()
	s.ApplyMarksConfig(cfg.Marks)
	s.ApplyRatingConfig(cfg.Rating)
	s.ApplyTaskerConfig(cfg.Tasker)
	return s
}

// ApplyMarksConfig overrides the mark settings with the config section.
func (s *RuntimeSettings) ApplyMarksConfig(cfg config.MarksConfig) {
	if cfg.DedupRadiusM > 0 {
		s.DedupRadiusM = int(cfg.DedupRadiusM)
	}
}

// ApplyRatingConfig overrides the rating settings with the config section.
func (s *RuntimeSettings) ApplyRatingConfig(cfg config.RatingConfig) {
	s.Rating = RatingSettings{
		CheckCorrect:  cfg.CheckCorrect,
		CheckWrong:    cfg.CheckWrong,
		MarkConfirmed: cfg.MarkConfirmed,
		MarkRefuted:   cfg.MarkRefuted,
		TaskCompleted: cfg.TaskCompleted,
	}
	if cfg.MaxChecksPerDay > 0 {
		s.MaxChecksPerDay = cfg.MaxChecksPerDay
	}
}

// ApplyTaskerConfig overrides the tasker settings with the config section.
func (s *RuntimeSettings) ApplyTaskerConfig(cfg config.TaskerConfig) {
	if cfg.MaxTasksPerUser > 0 {
		s.Tasker.MaxTasksPerUser = cfg.MaxTasksPerUser
	}
	if cfg.RequiredChecks > 0 {
		s.Tasker.RequiredChecks = cfg.RequiredChecks
	}
	// Zero is a valid (if degenerate) target: every mark counts as covered.
	if cfg.TargetProbability >= 0 {
		s.Tasker.TargetProbability = cfg.TargetProbability
	}
	if cfg.MaxRadiusMeters > 0 {
		s.Tasker.MaxRadiusMeters = cfg.MaxRadiusMeters
	}
	if cfg.TaskTTL > 0 {
		s.Tasker.TaskTTL = Duration(cfg.TaskTTL)
	}
}

// Validate checks that every value is inside its allowed range; the error
// wraps ErrInvalidArgument and lists every violation.
func (s RuntimeSettings) Validate() error {
	var errs []error
	check := func(ok bool, msg string) {
		if !ok {
			errs = append(errs, errors.New(msg))
		}
	}
	inRange := func(v, lo, hi int) bool { return v >= lo && v <= hi }

	check(inRange(s.VoteThreshold, 1, MaxVoteThreshold), fmt.Sprintf("vote_threshold must be in [1, %d]", MaxVoteThreshold))
	check(inRange(s.DedupRadiusM, 1, MaxDedupRadiusM), fmt.Sprintf("dedup_radius_m must be in [1, %d]", MaxDedupRadiusM))
	check(inRange(s.MaxChecksPerDay, 1, MaxChecksPerDayCap), fmt.Sprintf("max_checks_per_day must be in [1, %d]", MaxChecksPerDayCap))

	for name, v := range map[string]int{
		"rating.check_correct":  s.Rating.CheckCorrect,
		"rating.check_wrong":    s.Rating.CheckWrong,
		"rating.mark_confirmed": s.Rating.MarkConfirmed,
		"rating.mark_refuted":   s.Rating.MarkRefuted,
		"rating.task_completed": s.Rating.TaskCompleted,
	} {
		check(inRange(v, -MaxRatingDelta, MaxRatingDelta), fmt.Sprintf("%s must be in [%d, %d]", name, -MaxRatingDelta, MaxRatingDelta))
	}

	t := s.Tasker
	check(inRange(t.MaxTasksPerUser, 1, MaxTasksPerUserCap), fmt.Sprintf("tasker.max_tasks_per_user must be in [1, %d]", MaxTasksPerUserCap))
	check(inRange(t.RequiredChecks, 1, MaxRequiredChecks), fmt.Sprintf("tasker.required_checks must be in [1, %d]", MaxRequiredChecks))
	check(t.TargetProbability >= 0 && t.TargetProbability <= 1, "tasker.target_probability must be in [0, 1]")
	check(inRange(t.MaxRadiusMeters, 1, MaxTaskRadiusM), fmt.Sprintf("tasker.max_radius_meters must be in [1, %d]", MaxTaskRadiusM))
	check(t.TaskTTL >= Duration(time.Minute) && t.TaskTTL <= MaxTaskTTL, fmt.Sprintf("tasker.task_ttl must be in [1m, %s]", MaxTaskTTL))

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrInvalidArgument, errors.Join(errs...))
}

// SettingsProvider hands the current RuntimeSettings to the usecases that
// apply them (Checks, Updater, Marks, Tasker). Get never fails: an
// implementation falls back to the last known or the default values.
type SettingsProvider interface {
	Get(ctx context.Context) RuntimeSettings
}

// StaticSettings is a SettingsProvider that always returns the same value
// (config-only deployments and tests).
type StaticSettings RuntimeSettings

func (s StaticSettings) Get(context.Context) RuntimeSettings { return RuntimeSettings(s) }

// SettingsRepository stores the settings documents and their history.
type SettingsRepository interface {
	GetSetting(ctx context.Context, key string) (models.Setting, error)
	SetSetting(ctx context.Context, key string, value json.RawMessage, updatedBy null.Int) error
	GetSettingsHistory(ctx context.Context, key string, limit int) ([]models.SettingChange, error)
}

// Settings serves the admin settings: the database is the source of truth,
// the config provides the defaults for keys never written, and a per-process
// cache with SettingsCacheTTL keeps the hot paths cheap.
type Settings struct {
	log      *slog.Logger
	repo     SettingsRepository
	defaults RuntimeSettings
	ttl      time.Duration
	now      func() time.Time

	// snapshot is replaced atomically, so readers never block; refreshing
	// makes the refresh of an expired snapshot single-flight: the first
	// caller reloads, the others keep serving the stale value meanwhile.
	snapshot   atomic.Pointer[settingsSnapshot]
	refreshing atomic.Bool
}

type settingsSnapshot struct {
	settings  RuntimeSettings
	expiresAt time.Time
}

// NewSettings creates the settings service; defaults are returned until the
// first PUT and fill in fields missing from a stored document.
func NewSettings(log *slog.Logger, defaults RuntimeSettings, repo SettingsRepository) *Settings {
	uc := &Settings{
		log:      log,
		repo:     repo,
		defaults: defaults,
		ttl:      SettingsCacheTTL,
		now:      time.Now,
	}
	// Expired from the start: the first Get reads the database.
	uc.snapshot.Store(&settingsSnapshot{settings: defaults})
	return uc
}

// Get returns the current settings from the cache, refreshing it from the
// database when the TTL expired. A failed refresh logs the error and returns
// the last known value, so a database hiccup never changes behaviour.
func (uc *Settings) Get(ctx context.Context) RuntimeSettings {
	const op = "usecase.Settings.Get"

	current := uc.snapshot.Load()
	now := uc.now()
	if now.Before(current.expiresAt) || !uc.refreshing.CompareAndSwap(false, true) {
		return current.settings
	}
	defer uc.refreshing.Store(false)

	// Another caller may have refreshed between the Load and the CAS.
	if current = uc.snapshot.Load(); now.Before(current.expiresAt) {
		return current.settings
	}

	s, err := uc.load(ctx)
	if err != nil {
		uc.log.Error("failed to refresh settings, keeping the last known values", slog.String("op", op), logger.Err(err))
		s = current.settings
	}
	// A failure is cached for the TTL as well: no retry storm on a database
	// hiccup.
	uc.snapshot.Store(&settingsSnapshot{settings: s, expiresAt: now.Add(uc.ttl)})
	return s
}

// Load reads the settings straight from the database (defaults when never
// written), bypassing the cache; it serves GET /admin/settings.
func (uc *Settings) Load(ctx context.Context) (RuntimeSettings, error) {
	const op = "usecase.Settings.Load"

	s, err := uc.load(ctx)
	if err != nil {
		return RuntimeSettings{}, mapRepoErr(op, err)
	}
	return s, nil
}

func (uc *Settings) load(ctx context.Context) (RuntimeSettings, error) {
	s := uc.defaults
	stored, err := uc.repo.GetSetting(ctx, RuntimeSettingsKey)
	if err != nil {
		if Kind(mapRepoErr("", err)) == KindNotFound {
			return s, nil
		}
		return RuntimeSettings{}, err
	}
	// Unmarshal over the defaults so that a field added after the document
	// was written keeps its default.
	if err := json.Unmarshal(stored.Value, &s); err != nil {
		return RuntimeSettings{}, fmt.Errorf("decode stored settings: %w", err)
	}
	if err := s.Validate(); err != nil {
		return RuntimeSettings{}, fmt.Errorf("stored settings: %w", err)
	}
	return s, nil
}

// Update validates and stores the full settings document (PUT semantics:
// the client sends every field) on behalf of updatedBy, then drops the
// cache so the next Get sees the new values.
func (uc *Settings) Update(ctx context.Context, s RuntimeSettings, updatedBy int) (RuntimeSettings, error) {
	const op = "usecase.Settings.Update"

	if err := s.Validate(); err != nil {
		return RuntimeSettings{}, fmt.Errorf("%s: %w", op, err)
	}

	value, err := json.Marshal(s)
	if err != nil {
		return RuntimeSettings{}, fmt.Errorf("%s: %w", op, err)
	}

	by := null.IntFrom(int64(updatedBy))
	if updatedBy <= 0 {
		by = null.Int{}
	}
	if err := uc.repo.SetSetting(ctx, RuntimeSettingsKey, value, by); err != nil {
		return RuntimeSettings{}, mapRepoErr(op, err)
	}

	uc.snapshot.Store(&settingsSnapshot{settings: s, expiresAt: uc.now().Add(uc.ttl)})

	uc.log.Info("runtime settings updated", slog.String("op", op), slog.Int("updated_by", updatedBy))
	return s, nil
}

// History lists the latest changes of the runtime settings, newest first.
func (uc *Settings) History(ctx context.Context, limit int) ([]models.SettingChange, error) {
	const op = "usecase.Settings.History"

	if limit <= 0 || limit > MaxSettingsHistoryLimit {
		return nil, fmt.Errorf("%s: %w: limit must be in [1, %d]", op, ErrInvalidArgument, MaxSettingsHistoryLimit)
	}

	changes, err := uc.repo.GetSettingsHistory(ctx, RuntimeSettingsKey, limit)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return changes, nil
}

// MaxSettingsHistoryLimit caps the page of GET /admin/settings/history.
const MaxSettingsHistoryLimit = 100
