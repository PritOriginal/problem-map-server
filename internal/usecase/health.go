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
//
// A failed Required dependency makes the service not ready (ErrUnavailable).
// A failed Optional dependency is only reported: the service degrades
// gracefully without it (e.g. Redis, whose loss disables caching and rate
// limiting but not the API).
type HealthDependencies struct {
	Required map[string]Pinger
	Optional map[string]Pinger
}

// HealthReport maps a dependency name to HealthStatusOK or HealthStatusError.
type HealthReport map[string]string

// Health checks the connectivity of infrastructure dependencies. Results are
// cached for the configured TTL and concurrent checks are coalesced
// (single-flight), so probes on a public port cannot turn into a ping storm
// on the databases.
type Health struct {
	log      *slog.Logger
	deps     map[string]healthDep
	timeout  time.Duration
	cacheTTL time.Duration

	mu         sync.Mutex
	lastReport HealthReport
	lastErr    error
	lastAt     time.Time
	// inflight is the ping currently running, shared by every caller that
	// arrives before it finishes; nil when no ping is in progress.
	inflight *healthCall
}

type healthDep struct {
	pinger   Pinger
	optional bool
}

// healthCall is a single in-flight ping and its outcome once done is closed.
type healthCall struct {
	done   chan struct{}
	report HealthReport
	err    error
}

func NewHealth(log *slog.Logger, cfg config.HealthConfig, deps HealthDependencies) *Health {
	live := make(map[string]healthDep, len(deps.Required)+len(deps.Optional))
	for name, p := range deps.Optional {
		if !isNil(p) {
			live[name] = healthDep{pinger: p, optional: true}
		}
	}
	// Required wins if a name is listed in both.
	for name, p := range deps.Required {
		if !isNil(p) {
			live[name] = healthDep{pinger: p}
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
// returns ErrUnavailable when at least one required dependency failed.
//
// Concurrent callers share one ping instead of queueing behind each other.
// The ping runs detached from any caller's context (bounded only by the
// configured timeout), so a caller going away cannot poison the cache for
// everyone else; that caller just gets ctx.Err() back.
func (uc *Health) Check(ctx context.Context) (HealthReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	uc.mu.Lock()
	if uc.lastReport != nil && time.Since(uc.lastAt) < uc.cacheTTL {
		report, err := uc.lastReport, uc.lastErr
		uc.mu.Unlock()
		return report, err
	}

	call := uc.inflight
	if call == nil {
		call = &healthCall{done: make(chan struct{})}
		uc.inflight = call
		go uc.run(call) //nolint:gosec // G118: the ping is shared by all callers, so it must outlive any single request context
	}
	uc.mu.Unlock()

	select {
	case <-call.done:
		return call.report, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// run executes the ping for call, publishes the result into the cache and
// wakes every waiter.
func (uc *Health) run(call *healthCall) {
	call.report, call.err = uc.ping(context.Background())

	uc.mu.Lock()
	uc.lastReport, uc.lastErr = call.report, call.err
	uc.lastAt = time.Now()
	uc.inflight = nil
	uc.mu.Unlock()

	close(call.done)
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
		}(i, uc.deps[name].pinger)
	}
	wg.Wait()

	report := make(HealthReport, len(names))
	var err error
	for i, name := range names {
		report[name] = HealthStatusOK
		if errs[i] == nil {
			continue
		}
		report[name] = HealthStatusError
		if uc.deps[name].optional {
			uc.log.Warn("optional dependency unavailable, degrading",
				slog.String("op", op), slog.String("dependency", name), slogger.Err(errs[i]))
			continue
		}
		uc.log.Warn("readiness check failed",
			slog.String("op", op), slog.String("dependency", name), slogger.Err(errs[i]))
		err = ErrUnavailable
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
