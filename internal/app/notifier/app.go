// Package notifier is the worker that turns domain events (NATS) into
// notifications: it subscribes to mark.status_changed, task.assigned and
// check.added, stores a notification per addressee and hands it to the
// PushSender.
package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	natsgo "github.com/nats-io/nats.go"
)

// Subscriber is the part of the NATS client the worker uses; it is an
// interface so the dispatch logic can be tested without a broker.
type Subscriber interface {
	Subscribe(subject string, handler func(ctx context.Context, data []byte) error) (*natsgo.Subscription, error)
}

// Handlers are the event handlers the worker dispatches to.
type Handlers interface {
	HandleMarkStatusChanged(ctx context.Context, ev events.MarkStatusChanged) error
	HandleTaskAssigned(ctx context.Context, ev events.TaskAssigned) error
	HandleCheckAdded(ctx context.Context, ev events.CheckAdded) error
}

// App is the notifier worker: Run subscribes and blocks until Stop.
type App struct {
	log     *slog.Logger
	nats    *nats.Client
	closers app.Closers
	router  *Router

	stopOnce sync.Once
	done     chan struct{}
}

func New(log *slog.Logger, cfg *config.Config) *App {
	// Clients are registered in dependency order; app.Closers closes them in
	// reverse (nats -> database).
	var closers app.Closers

	postgresDB, err := postgres.New(cfg.DB)
	if err != nil {
		log.Error("failed connection to database", slogger.Err(err))
		panic(err)
	}
	log.Info("PostgreSQL connected!")
	closers.Add("database", postgresDB)

	natsClient, err := nats.New(log, cfg.Nats)
	if err != nil {
		log.Error("failed connection to nats", slogger.Err(err))
		panic(err)
	}
	log.Info("NATS connected!")
	closers.Add("nats", natsClient)

	notificationsRepo := postgres.NewNotifications(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)

	// Extension point: replace NewLogPushSender with a real provider
	// (FCM/APNs) implementing usecase.PushSender.
	notificationsUseCase := usecase.NewNotifications(log, usecase.NewLogPushSender(log), usecase.NotificationsRepositories{
		Notifications: notificationsRepo,
		Devices:       notificationsRepo,
	})
	notifier := usecase.NewNotifier(log, notificationsUseCase, usecase.NotifierRepositories{
		Marks: marksRepo,
	})

	return &App{
		log:     log,
		nats:    natsClient,
		closers: closers,
		router:  NewRouter(log, notifier),
		done:    make(chan struct{}),
	}
}

// Run subscribes to the event subjects and blocks until Stop is called.
func (a *App) Run() error {
	const op = "notifier.Run"

	if err := a.router.Subscribe(a.nats); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	a.log.Info("notifier started", slog.Any("subjects", a.router.Subjects()))

	<-a.done
	return nil
}

// Stop unblocks Run and closes the clients (NATS, then PostgreSQL).
func (a *App) Stop() {
	const op = "notifier.Stop"

	log := a.log.With(slog.String("op", op))
	log.Info("stopping notifier")

	a.stopOnce.Do(func() { close(a.done) })
	a.closers.Close(log)
}

// Router decodes raw event payloads and dispatches them to Handlers.
type Router struct {
	log      *slog.Logger
	handlers Handlers
}

func NewRouter(log *slog.Logger, handlers Handlers) *Router {
	return &Router{log: log, handlers: handlers}
}

// Subjects lists the subjects the router consumes.
func (r *Router) Subjects() []string {
	return []string{events.SubjectMarkStatusChanged, events.SubjectTaskAssigned, events.SubjectCheckAdded}
}

// Subscribe registers Handle for every subject on sub.
func (r *Router) Subscribe(sub Subscriber) error {
	for _, subject := range r.Subjects() {
		subject := subject
		if _, err := sub.Subscribe(subject, func(ctx context.Context, data []byte) error {
			return r.Handle(ctx, subject, data)
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
	}
	return nil
}

// Handle decodes data as the event of subject and runs its handler. An
// unknown subject or an undecodable payload is an error (and is logged by
// the subscription), never a panic.
func (r *Router) Handle(ctx context.Context, subject string, data []byte) error {
	switch subject {
	case events.SubjectMarkStatusChanged:
		return handle(ctx, subject, data, r.handlers.HandleMarkStatusChanged)
	case events.SubjectTaskAssigned:
		return handle(ctx, subject, data, r.handlers.HandleTaskAssigned)
	case events.SubjectCheckAdded:
		return handle(ctx, subject, data, r.handlers.HandleCheckAdded)
	default:
		return fmt.Errorf("unknown subject %q", subject)
	}
}

func handle[T any](ctx context.Context, subject string, data []byte, fn func(context.Context, T) error) error {
	var ev T
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("decode %s: %w", subject, err)
	}
	if err := fn(ctx, ev); err != nil {
		return fmt.Errorf("handle %s: %w", subject, err)
	}
	return nil
}
