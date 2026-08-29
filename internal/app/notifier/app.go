// Package notifier is the worker that turns domain events (NATS) into
// notifications: it consumes mark.status_changed, task.assigned and
// check.added from the JetStream stream, stores a notification per
// addressee and hands it to the PushSender. Every event is acknowledged
// only after it was handled, so a crash or a database outage never loses a
// notification; a poison event ends up in the dead-letter stream.
package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/app"
	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/middleware/metrics"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/internal/push/apns"
	"github.com/PritOriginal/problem-map-server/internal/push/fcm"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
)

// ConsumerName is the durable JetStream consumer (and the core NATS queue
// group) of the worker: every event is handled by exactly one running
// notifier instance and the position survives restarts.
const ConsumerName = "notifier"

// Handlers are the event handlers the worker dispatches to.
type Handlers interface {
	HandleMarkStatusChanged(ctx context.Context, ev events.MarkStatusChanged) error
	HandleTaskAssigned(ctx context.Context, ev events.TaskAssigned) error
	HandleCheckAdded(ctx context.Context, ev events.CheckAdded) error
}

// App is the notifier worker: Run consumes and blocks until Stop.
type App struct {
	log     *slog.Logger
	nats    *nats.Client
	closers app.Closers
	router  *Router
	// metricsServer serves the Prometheus registry over HTTP; nil when
	// notifier.metrics-port is 0.
	metricsServer   *http.Server
	shutdownTimeout time.Duration

	mu       sync.Mutex
	consumer *nats.Consumer
	stopped  bool
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

	// Same registry layout as the servers (Go/process collectors included).
	m := metrics.New()

	natsClient, err := nats.New(log, cfg.Nats, nats.WithMetrics(nats.NewMetrics(m.Registry())))
	if err != nil {
		log.Error("failed connection to nats", slogger.Err(err))
		panic(err)
	}
	log.Info("NATS connected!", slog.Bool("jetstream", natsClient.JetStream()))
	closers.Add("nats", natsClient)

	notificationsRepo := postgres.NewNotifications(postgresDB.DB, trmsqlx.DefaultCtxGetter)
	marksRepo := postgres.NewMarks(postgresDB.DB, trmsqlx.DefaultCtxGetter)

	pushMetrics := push.NewMetrics(m.Registry())

	notificationsUseCase := usecase.NewNotifications(log, newPushSender(log, cfg.Push), usecase.NotificationsRepositories{
		Notifications: notificationsRepo,
		Devices:       notificationsRepo,
	}, usecase.WithPushMetrics(pushMetrics), usecase.WithPushTimeout(cfg.Push.SendTimeout))
	notifier := usecase.NewNotifier(log, notificationsUseCase, usecase.NotifierRepositories{
		Marks: marksRepo,
	})

	var metricsServer *http.Server
	if cfg.Notifier.MetricsPort > 0 {
		metricsServer = m.Server(":" + strconv.Itoa(cfg.Notifier.MetricsPort))
	}

	return &App{
		log:             log,
		nats:            natsClient,
		closers:         closers,
		router:          NewRouter(log, notifier),
		metricsServer:   metricsServer,
		shutdownTimeout: cfg.ShutdownTimeout,
		done:            make(chan struct{}),
	}
}

// newPushSender wires the providers configured in cfg: FCM for android and
// web, the APNs stub for ios. Without any credentials every push is only
// logged (push.LogSender).
func newPushSender(log *slog.Logger, cfg config.PushConfig) usecase.PushSender {
	if !cfg.FCM.Enabled() && !cfg.APNs.Enabled() {
		log.Warn("push credentials are not configured: notifications are only logged, not delivered")
		return push.NewLogSender(log)
	}

	senders := make(map[models.DevicePlatform]push.Sender, 3)
	if cfg.FCM.Enabled() {
		fcmSender, err := fcm.New(log, cfg.FCM)
		if err != nil {
			log.Error("failed to init FCM sender", slogger.Err(err))
			panic(err)
		}
		senders[models.PlatformAndroid] = fcmSender
		senders[models.PlatformWeb] = fcmSender
		log.Info("FCM push sender enabled")
	} else {
		log.Warn("FCM is not configured: android and web pushes are only logged")
	}
	if cfg.APNs.Enabled() {
		senders[models.PlatformIOS] = apns.New(log, cfg.APNs)
		log.Warn("APNs push sender is a stub: ios pushes are only logged")
	}

	return push.NewMulti(push.NewLogSender(log), senders)
}

// Run starts the consumer and the metrics server (when configured) and
// blocks until Stop is called. A failure of the metrics server is logged
// but does not stop the worker.
func (a *App) Run() error {
	const op = "notifier.Run"

	if !a.startConsumer() {
		return nil
	}
	a.log.Info("notifier started",
		slog.String("consumer", ConsumerName),
		slog.Bool("jetstream", a.nats.JetStream()),
		slog.Any("subjects", a.router.Subjects()),
	)

	if a.metricsServer != nil {
		go func() {
			a.log.Info("notifier metrics server started", slog.String("address", a.metricsServer.Addr))
			if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.log.Error("notifier metrics server failed", slogger.Err(err))
			}
		}()
	}

	select {
	case <-a.done:
		return nil
	case <-a.nats.Closed():
		// The connection is gone for good (not a reconnect): the worker
		// cannot receive events any more, so it exits instead of idling.
		return fmt.Errorf("%s: nats connection closed", op)
	}
}

// startConsumer subscribes unless Stop already ran (a signal during
// startup); the error of a failed subscription is reported through Run.
func (a *App) startConsumer() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return false
	}

	consumer, err := a.nats.Consume(context.Background(), nats.ConsumerConfig{
		Name:     ConsumerName,
		Subjects: a.router.Subjects(),
	}, a.router.Handle)
	if err != nil {
		a.log.Error("failed to start consumer", slogger.Err(err))
		panic(err)
	}
	a.consumer = consumer
	return true
}

// Stop unblocks Run, drains the consumer (in-flight events finish and are
// acknowledged), shuts the metrics server down and closes the clients
// (NATS, then PostgreSQL), all within the configured shutdown timeout.
func (a *App) Stop() {
	const op = "notifier.Stop"

	log := a.log.With(slog.String("op", op))
	log.Info("stopping notifier")

	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	close(a.done)
	consumer := a.consumer
	a.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()

	if consumer != nil {
		if err := consumer.Stop(shutdownCtx); err != nil {
			log.Error("an error occurred while draining the consumer", slogger.Err(err))
		}
	}
	if a.metricsServer != nil {
		if err := a.metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error("an error occurred while stopping the metrics server", slogger.Err(err))
		}
	}

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

// Handle decodes data as the event of subject and runs its handler
// (nats.MsgHandler). An unknown subject, an undecodable payload or a
// payload of a newer schema version is a nats.ErrNoRetry error (the
// event is dead-lettered, a redelivery cannot fix it); a handler error is
// returned as is so that the event is redelivered.
func (r *Router) Handle(ctx context.Context, subject string, data []byte) error {
	switch subject {
	case events.SubjectMarkStatusChanged:
		return handle(ctx, subject, data, r.handlers.HandleMarkStatusChanged)
	case events.SubjectTaskAssigned:
		return handle(ctx, subject, data, r.handlers.HandleTaskAssigned)
	case events.SubjectCheckAdded:
		return handle(ctx, subject, data, r.handlers.HandleCheckAdded)
	default:
		return fmt.Errorf("%w: unknown subject %q", nats.ErrNoRetry, subject)
	}
}

// versioned is implemented by every event via the embedded events.Header.
type versioned interface {
	CheckVersion() error
}

func handle[T versioned](ctx context.Context, subject string, data []byte, fn func(context.Context, T) error) error {
	var ev T
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("%w: decode %s: %w", nats.ErrNoRetry, subject, err)
	}
	if err := ev.CheckVersion(); err != nil {
		return fmt.Errorf("%w: decode %s: %w", nats.ErrNoRetry, subject, err)
	}
	if err := fn(ctx, ev); err != nil {
		return fmt.Errorf("handle %s: %w", subject, err)
	}
	return nil
}
