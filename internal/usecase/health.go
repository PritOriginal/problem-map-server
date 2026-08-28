package usecase

import (
	"context"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

const (
	HealthStatusOK    = "ok"
	HealthStatusError = "error"
)

// Pinger is implemented by infrastructure clients that can verify connectivity.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthDependencies lists the named dependencies checked by the readiness
// probe. Nil values (including typed nil pointers) are skipped.
type HealthDependencies map[string]Pinger

// HealthReport maps a dependency name to HealthStatusOK or HealthStatusError.
type HealthReport map[string]string

// Health checks the connectivity of infrastructure dependencies. Results are
// cached for the configured TTL and concurrent checks are coalesced, so probes
// on a public port cannot turn into a ping storm on the databases.
type Health struct {
	log      *slog.Logger
	deps     HealthDependencies
	timeout  time.Duration
	cacheTTL time.Duration

	mu         sync.Mutex
	lastReport HealthReport
	lastErr    error
	lastAt     time.Time
}

func NewHealth(log *slog.Logger, cfg config.HealthConfig, deps HealthDependencies) *Health {
	live := make(HealthDependencies, len(deps))
	for name, p := range deps {
		if !isNil(p) {
			live[name] = p
		}
	}

	return &Health{
		log:      log,
		deps:     live,
		timeout:  cfg.Timeout,
		cacheTTL: cfg.CacheTTL,
	}
}

// Check pings every dependency and returns the per-dependency report. It
// returns ErrUnavailable when at least one dependency failed.
func (uc *Health) Check(ctx context.Context) (HealthReport, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if uc.lastReport != nil && time.Since(uc.lastAt) < uc.cacheTTL {
		return uc.lastReport, uc.lastErr
	}

	uc.lastReport, uc.lastErr = uc.ping(ctx)
	uc.lastAt = time.Now()
	return uc.lastReport, uc.lastErr
}

// ping checks all dependencies concurrently so each gets the full timeout
// budget and the total latency is bounded by the slowest, not the sum.
func (uc *Health) ping(ctx context.Context) (HealthReport, error) {
	const op = "usecase.Health.Check"

	ctx, cancel := context.WithTimeout(ctx, uc.timeout)
	defer cancel()

	names := make([]string, 0, len(uc.deps))
	for name := range uc.deps {
		names = append(names, name)
	}

	errs := make([]error, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, p Pinger) {
			defer wg.Done()
			errs[i] = p.Ping(ctx)
		}(i, uc.deps[name])
	}
	wg.Wait()

	report := make(HealthReport, len(names))
	var err error
	for i, name := range names {
		report[name] = HealthStatusOK
		if errs[i] != nil {
			uc.log.Warn("readiness check failed",
				slog.String("op", op), slog.String("dependency", name), slogger.Err(errs[i]))
			report[name] = HealthStatusError
			err = ErrUnavailable
		}
	}
	return report, err
}

func isNil(p Pinger) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
