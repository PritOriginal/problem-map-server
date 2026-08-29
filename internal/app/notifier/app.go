// Package notifier is the worker that turns domain events (NATS) into
// notifications: it subscribes to mark.status_changed, task.assigned,
// check.added, mark.assigned and mark.sla_breached, stores a notification per addressee and hands it to the
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
	QueueSubscribe(subject, queue string, handler nats.Handler) (*natsgo.Subscription, error)
}

// QueueGroup is the NATS queue group of the worker: every event is handled
// by exactly one running notifier instance, so the worker scales out
// without duplicating notifications.
const QueueGroup = "notifier"

// Handlers are the event handlers the worker dispatches to.
type Handlers interface {
	HandleMarkStatusChanged(ctx context.Context, ev events.MarkStatusChanged) error
	HandleTaskAssigned(ctx context.Context, ev events.TaskAssigned) error
	HandleCheckAdded(ctx context.Context, ev events.CheckAdded) error
	HandleMarkAssigned(ctx context.Context, ev events.MarkAssigned) error
	HandleMarkSLABreached(ctx context.Context, ev events.MarkSLABreached) error
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
		Marks:         marksRepo,
		Organizations: postgres.NewOrganizations(postgresDB.DB, trmsqlx.DefaultCtxGetter),
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

	select {
	case <-a.done:
		return nil
	case <-a.nats.Closed():
		// The connection is gone for good (not a reconnect): the worker
		// cannot receive events any more, so it exits instead of idling.
		return fmt.Errorf("%s: nats connection closed", op)
	}
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
	return []string{
		events.SubjectMarkStatusChanged, events.SubjectTaskAssigned, events.SubjectCheckAdded,
		events.SubjectMarkAssigned, events.SubjectMarkSLABreached,
	}
}

// Subscribe registers Handle for every subject on sub within QueueGroup.
func (r *Router) Subscribe(sub Subscriber) error {
	for _, subject := range r.Subjects() {
		if _, err := sub.QueueSubscribe(subject, QueueGroup, func(ctx context.Context, data []byte) error {
			return r.Handle(ctx, subject, data)
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
	}
	return nil
}

// Handle decodes data as the event of subject and runs its handler. An
// unknown subject, an undecodable payload or a payload of a newer schema
// version is an error (and is logged by the subscription), never a panic.
func (r *Router) Handle(ctx context.Context, subject string, data []byte) error {
	switch subject {
	case events.SubjectMarkStatusChanged:
		return handle(ctx, subject, data, r.handlers.HandleMarkStatusChanged)
	case events.SubjectTaskAssigned:
		return handle(ctx, subject, data, r.handlers.HandleTaskAssigned)
	case events.SubjectCheckAdded:
		return handle(ctx, subject, data, r.handlers.HandleCheckAdded)
	case events.SubjectMarkAssigned:
		return handle(ctx, subject, data, r.handlers.HandleMarkAssigned)
	case events.SubjectMarkSLABreached:
		return handle(ctx, subject, data, r.handlers.HandleMarkSLABreached)
	default:
		return fmt.Errorf("unknown subject %q", subject)
	}
}

// versioned is implemented by every event via the embedded events.Header.
type versioned interface {
	CheckVersion() error
}

func handle[T versioned](ctx context.Context, subject string, data []byte, fn func(context.Context, T) error) error {
	var ev T
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("decode %s: %w", subject, err)
	}
	if err := ev.CheckVersion(); err != nil {
		return fmt.Errorf("decode %s: %w", subject, err)
	}
	if err := fn(ctx, ev); err != nil {
		return fmt.Errorf("handle %s: %w", subject, err)
	}
	return nil
}
