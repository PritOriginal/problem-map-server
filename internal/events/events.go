// Package events defines the domain events published by the servers and
// consumed by background workers (cmd/notifier), together with the Publisher
// abstraction that decouples use cases from the message broker.
package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/google/uuid"
)

// SchemaVersion is the version of the event payloads written into the
// "v" field. Bump it on an incompatible change of a payload; consumers
// reject payloads of an unknown (newer) version instead of guessing.
const SchemaVersion = 1

// Subjects (NATS) of the domain events.
const (
	SubjectMarkStatusChanged = "mark.status_changed"
	SubjectTaskAssigned      = "task.assigned"
	SubjectCheckAdded        = "check.added"
	SubjectMarkAssigned      = "mark.assigned"
	SubjectMarkSLABreached   = "mark.sla_breached"
)

// Publisher sends a domain event to the broker. Implementations must be
// safe for concurrent use.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload any) error
}

// Event is implemented by every domain event; Subject reports where the
// event is published.
type Event interface {
	Subject() string
}

// Header is embedded into every event payload.
type Header struct {
	// Version is the payload schema version (SchemaVersion at publish time).
	Version int    `json:"v"`
	EventID string `json:"event_id"`
}

func newHeader() Header {
	return Header{Version: SchemaVersion, EventID: uuid.NewString()}
}

// CheckVersion reports an error when the payload was written by a newer
// schema than this binary understands.
func (h Header) CheckVersion() error {
	if h.Version > SchemaVersion {
		return fmt.Errorf("%w: got %d, supported up to %d", ErrUnsupportedVersion, h.Version, SchemaVersion)
	}
	return nil
}

// ErrUnsupportedVersion is returned by Header.CheckVersion for a payload of
// a newer schema.
var ErrUnsupportedVersion = errors.New("unsupported event schema version")

// MarkStatusChanged is published after a mark moved to a new status.
type MarkStatusChanged struct {
	Header
	MarkID    int                   `json:"mark_id"`
	OldStatus models.MarkStatusType `json:"old_status"`
	NewStatus models.MarkStatusType `json:"new_status"`
	// AuthorID is the user who created the mark.
	AuthorID int `json:"author_id"`
}

func (MarkStatusChanged) Subject() string { return SubjectMarkStatusChanged }

// NewMarkStatusChanged builds the event with a fresh EventID.
func NewMarkStatusChanged(markID int, oldStatus, newStatus models.MarkStatusType, authorID int) MarkStatusChanged {
	return MarkStatusChanged{
		Header:    newHeader(),
		MarkID:    markID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		AuthorID:  authorID,
	}
}

// TaskAssigned is published after a verification task was issued to a user.
type TaskAssigned struct {
	Header
	TaskID int        `json:"task_id"`
	UserID int        `json:"user_id"`
	MarkID int        `json:"mark_id"`
	DueAt  *time.Time `json:"due_at,omitempty"`
}

func (TaskAssigned) Subject() string { return SubjectTaskAssigned }

// NewTaskAssigned builds the event with a fresh EventID.
func NewTaskAssigned(taskID, userID, markID int, dueAt *time.Time) TaskAssigned {
	return TaskAssigned{
		Header: newHeader(),
		TaskID: taskID,
		UserID: userID,
		MarkID: markID,
		DueAt:  dueAt,
	}
}

// CheckAdded is published after a user submitted a check for a mark.
type CheckAdded struct {
	Header
	CheckID int `json:"check_id"`
	MarkID  int `json:"mark_id"`
	// UserID is the author of the check.
	UserID int `json:"user_id"`
}

func (CheckAdded) Subject() string { return SubjectCheckAdded }

// NewCheckAdded builds the event with a fresh EventID.
func NewCheckAdded(checkID, markID, userID int) CheckAdded {
	return CheckAdded{
		Header:  newHeader(),
		CheckID: checkID,
		MarkID:  markID,
		UserID:  userID,
	}
}

// MarkAssigned is published after a mark was assigned to an organization
// (automatically on confirmation or by a moderator).
type MarkAssigned struct {
	Header
	MarkID         int       `json:"mark_id"`
	OrganizationID int       `json:"organization_id"`
	SLADueAt       time.Time `json:"sla_due_at"`
}

func (MarkAssigned) Subject() string { return SubjectMarkAssigned }

// NewMarkAssigned builds the event with a fresh EventID.
func NewMarkAssigned(markID, organizationID int, slaDueAt time.Time) MarkAssigned {
	return MarkAssigned{
		Header:         newHeader(),
		MarkID:         markID,
		OrganizationID: organizationID,
		SLADueAt:       slaDueAt,
	}
}

// MarkSLABreached is published when the SLA deadline of an assigned mark
// has passed while it is still confirmed or in progress. The EventID is
// derived from (mark_id, sla_due_at), so the periodic SLA check may publish
// it again without producing duplicate notifications.
type MarkSLABreached struct {
	Header
	MarkID         int       `json:"mark_id"`
	OrganizationID int       `json:"organization_id"`
	SLADueAt       time.Time `json:"sla_due_at"`
}

func (MarkSLABreached) Subject() string { return SubjectMarkSLABreached }

// slaBreachNamespace is the UUID namespace of MarkSLABreached event ids.
var slaBreachNamespace = uuid.MustParse("6b8e0f3a-5a1f-4c8e-9d2b-7f0c4a1e2d91")

// NewMarkSLABreached builds the event with a deterministic EventID.
func NewMarkSLABreached(markID, organizationID int, slaDueAt time.Time) MarkSLABreached {
	slaDueAt = slaDueAt.UTC()
	id := uuid.NewSHA1(slaBreachNamespace, []byte(fmt.Sprintf("%d:%s", markID, slaDueAt.Format(time.RFC3339Nano))))
	return MarkSLABreached{
		Header:         Header{Version: SchemaVersion, EventID: id.String()},
		MarkID:         markID,
		OrganizationID: organizationID,
		SLADueAt:       slaDueAt,
	}
}

// PublishEvent publishes ev on its own subject and logs (but does not
// return) a failure: events are best-effort side effects and must never
// fail the business operation that produced them.
func PublishEvent(ctx context.Context, log *slog.Logger, p Publisher, ev Event) {
	if p == nil {
		return
	}
	if err := p.Publish(ctx, ev.Subject(), ev); err != nil {
		// The event is lost (best effort); it is logged in full so it can
		// be replayed by hand if needed.
		log.Warn("failed to publish event, event lost",
			slog.String("subject", ev.Subject()),
			slog.Any("event", ev),
			slog.String("error", err.Error()),
		)
	}
}

// NoopPublisher discards every event. It is used in tests and when no
// broker is configured.
type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, string, any) error { return nil }

// Pending collects events produced inside a transaction so that they can
// be published only after the transaction committed. It is not safe for
// concurrent use; one Pending per operation.
type Pending struct {
	events []Event
}

// Add queues ev.
func (p *Pending) Add(ev Event) { p.events = append(p.events, ev) }

// Events returns the queued events in order.
func (p *Pending) Events() []Event { return p.events }

// Flush publishes every queued event via pub and clears the queue.
func (p *Pending) Flush(ctx context.Context, log *slog.Logger, pub Publisher) {
	for _, ev := range p.events {
		PublishEvent(ctx, log, pub, ev)
	}
	p.events = nil
}

type pendingKey struct{}

// WithPending returns a context carrying p. Code that produces events
// inside a transaction queues them via Collect instead of publishing.
func WithPending(ctx context.Context, p *Pending) context.Context {
	return context.WithValue(ctx, pendingKey{}, p)
}

// Collect queues ev on the Pending stored in ctx and reports true, or
// reports false when ctx carries no Pending so the caller publishes
// directly.
func Collect(ctx context.Context, ev Event) bool {
	p, ok := ctx.Value(pendingKey{}).(*Pending)
	if !ok || p == nil {
		return false
	}
	p.Add(ev)
	return true
}
