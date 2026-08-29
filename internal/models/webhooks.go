package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guregu/null/v6"
)

// Webhook is an HTTP subscription to domain events. Secret is never
// serialised: it is shown once, in the response of the creating request.
type Webhook struct {
	ID          int    `json:"webhook_id" db:"webhook_id"`
	OwnerUserID int    `json:"owner_user_id" db:"owner_user_id"`
	URL         string `json:"url" db:"url"`
	Secret      string `json:"-" db:"secret"`
	// Events lists the subjects the webhook receives; "mark.*" matches
	// every subject of the prefix and "*" every event.
	Events    []string  `json:"events" db:"-"`
	Active    bool      `json:"active" db:"active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// WebhookUpdate lists the webhook fields a client may change; nil means "keep".
type WebhookUpdate struct {
	Active *bool
	Events []string
}

// IsEmpty reports whether the update changes nothing.
func (u WebhookUpdate) IsEmpty() bool {
	return u.Active == nil && u.Events == nil
}

// WebhookEventAll subscribes a webhook to every event.
const WebhookEventAll = "*"

// MatchesEvent reports whether the subscription covers subject: an exact
// match, a "prefix.*" pattern or "*".
func (w Webhook) MatchesEvent(subject string) bool {
	return WebhookEventsMatch(w.Events, subject)
}

// WebhookEventsMatch reports whether events cover subject (see
// Webhook.MatchesEvent).
func WebhookEventsMatch(events []string, subject string) bool {
	prefix, _, _ := strings.Cut(subject, ".")
	for _, e := range events {
		if e == subject || e == WebhookEventAll || e == prefix+".*" {
			return true
		}
	}
	return false
}

// WebhookSubjectTest is the subject of the event sent by POST /webhooks/{id}/test.
const WebhookSubjectTest = "webhook.test"

// MaxWebhookAttempts is the number of delivery attempts (the first one plus
// the retries of WebhookBackoff) after which the webhook is deactivated.
const MaxWebhookAttempts = 6

// webhookBackoff lists the delay before retry n (n = 1..5).
var webhookBackoff = [...]time.Duration{
	time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	12 * time.Hour,
}

// WebhookBackoff returns the delay before the next attempt after failedAttempt
// (1-based) and false when no further attempt is made.
func WebhookBackoff(failedAttempt int) (time.Duration, bool) {
	if failedAttempt < 1 || failedAttempt > len(webhookBackoff) {
		return 0, false
	}
	return webhookBackoff[failedAttempt-1], true
}

// WebhookDelivery is the delivery state of one event to one webhook. The
// row is updated in place on every attempt.
type WebhookDelivery struct {
	ID        int64           `json:"delivery_id" db:"delivery_id"`
	WebhookID int             `json:"webhook_id" db:"webhook_id"`
	EventID   string          `json:"event_id" db:"event_id"`
	Subject   string          `json:"subject" db:"subject"`
	Payload   json.RawMessage `json:"payload" db:"payload" swaggertype:"object"`
	// Attempt is the number of attempts made so far.
	Attempt       int         `json:"attempt" db:"attempt"`
	StatusCode    null.Int    `json:"status_code" db:"status_code" swaggertype:"integer"`
	Error         null.String `json:"error" db:"error" swaggertype:"string"`
	DeliveredAt   null.Time   `json:"delivered_at" db:"delivered_at" swaggertype:"string" format:"date-time"`
	NextAttemptAt null.Time   `json:"next_attempt_at" db:"next_attempt_at" swaggertype:"string" format:"date-time"`
	CreatedAt     time.Time   `json:"created_at" db:"created_at"`
}

// Delivered reports whether the delivery succeeded.
func (d WebhookDelivery) Delivered() bool { return d.DeliveredAt.Valid }

// WebhookAttemptResult records the outcome of one delivery attempt.
type WebhookAttemptResult struct {
	Attempt    int
	StatusCode null.Int
	Error      null.String
	// DeliveredAt is set on success; NextAttemptAt when another attempt is
	// scheduled. Both empty means the delivery is given up.
	DeliveredAt   null.Time
	NextAttemptAt null.Time
}

// PendingWebhookDelivery is a delivery due for another attempt together
// with its webhook.
type PendingWebhookDelivery struct {
	Delivery WebhookDelivery
	Webhook  Webhook
}

// WebhookPayload is the JSON body POSTed to a webhook.
type WebhookPayload struct {
	EventID    string          `json:"event_id"`
	Subject    string          `json:"subject"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data" swaggertype:"object"`
}

// Webhook URL/event validation errors (wrapped into usecase.ErrInvalidArgument).
var (
	ErrInvalidWebhookURL    = errors.New("invalid webhook url")
	ErrInvalidWebhookEvents = errors.New("invalid webhook events")
)

// MaxWebhookEvents caps the subscriptions of one webhook.
const MaxWebhookEvents = 32

// ValidateWebhookEvents checks that events is a non-empty list of known
// subjects or patterns ("*", "mark.*"). known lists the exact subjects.
func ValidateWebhookEvents(events []string, known []string) error {
	if len(events) == 0 {
		return fmt.Errorf("%w: at least one event is required", ErrInvalidWebhookEvents)
	}
	if len(events) > MaxWebhookEvents {
		return fmt.Errorf("%w: at most %d events", ErrInvalidWebhookEvents, MaxWebhookEvents)
	}
	for _, e := range events {
		if e == WebhookEventAll {
			continue
		}
		matched := false
		for _, k := range known {
			if e == k || (strings.HasSuffix(e, ".*") && strings.HasPrefix(k, strings.TrimSuffix(e, "*"))) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: unknown event %q", ErrInvalidWebhookEvents, e)
		}
	}
	return nil
}
