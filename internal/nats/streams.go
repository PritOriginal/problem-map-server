package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// JetStream streams of the domain events.
const (
	// StreamEvents captures every domain event (mark.>, task.>, check.>).
	StreamEvents = "PROBLEM_MAP_EVENTS"
	// StreamDLQ receives the events a consumer gave up on (dead-letter
	// queue); see Consumer.
	StreamDLQ = "PROBLEM_MAP_DLQ"
	// DLQSubjectPrefix is prepended to the original subject of a
	// dead-lettered event: mark.status_changed -> dlq.mark.status_changed.
	DLQSubjectPrefix = "dlq."

	// eventsMaxAge is how long an event is kept in StreamEvents. Retention
	// is limits-based (not work-queue) on purpose: a work-queue stream
	// allows a single consumer per subject, deletes an event as soon as it
	// is acked (no replay for debugging or for a new consumer) and keeps a
	// message that exhausted MaxDeliver forever. With limits every consumer
	// tracks its own position, an event stays replayable for a week and
	// the stream cleans itself up.
	eventsMaxAge = 7 * 24 * time.Hour
	// dlqMaxAge is how long a dead-lettered event waits for an operator.
	dlqMaxAge = 30 * 24 * time.Hour
	// duplicatesWindow is the server-side deduplication window for
	// Nats-Msg-Id (the event_id): a retried publish of the same event
	// inside the window is dropped by the server.
	duplicatesWindow = 2 * time.Minute
)

// EventSubjects are the subject filters of StreamEvents.
var EventSubjects = []string{"mark.>", "task.>", "check.>"}

// Headers set on a dead-lettered message.
const (
	HeaderDLQSubject    = "X-Original-Subject"
	HeaderDLQStream     = "X-Original-Stream"
	HeaderDLQStreamSeq  = "X-Original-Stream-Seq"
	HeaderDLQConsumer   = "X-Consumer"
	HeaderDLQDeliveries = "X-Deliveries"
	HeaderDLQError      = "X-Error"
	// HeaderDLQMsgID keeps the original Nats-Msg-Id (the event_id): the
	// copy gets its own id so it is deduplicated per consumer, not against
	// the original.
	HeaderDLQMsgID = "X-Original-Msg-Id"
)

// ensureStreams creates (or brings up to date) StreamEvents and StreamDLQ.
// It is idempotent and runs on every start of a publisher and of a
// consumer, so the stream configuration in this file is the source of
// truth.
func ensureStreams(ctx context.Context, js jetstream.JetStream) error {
	streams := []jetstream.StreamConfig{
		{
			Name:        StreamEvents,
			Description: "problem-map domain events",
			Subjects:    EventSubjects,
			Retention:   jetstream.LimitsPolicy,
			Storage:     jetstream.FileStorage,
			MaxAge:      eventsMaxAge,
			Duplicates:  duplicatesWindow,
		},
		{
			Name:        StreamDLQ,
			Description: "problem-map domain events that exhausted their deliveries",
			Subjects:    []string{DLQSubjectPrefix + ">"},
			Retention:   jetstream.LimitsPolicy,
			Storage:     jetstream.FileStorage,
			MaxAge:      dlqMaxAge,
			Duplicates:  duplicatesWindow,
		},
	}
	for _, cfg := range streams {
		if err := ensureStream(ctx, js, cfg); err != nil {
			return fmt.Errorf("ensure stream %s: %w", cfg.Name, err)
		}
	}
	return nil
}

// ensureStream is CreateOrUpdateStream made safe against a concurrent
// start: when two processes (REST and the notifier) both find the stream
// missing and race to create it, the loser gets
// ErrStreamNameAlreadyInUse (the configs differ, e.g. across a rolling
// upgrade) and simply updates the stream the winner created.
func ensureStream(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) error {
	_, err := js.CreateOrUpdateStream(ctx, cfg)
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		_, err = js.UpdateStream(ctx, cfg)
	}
	return err
}
