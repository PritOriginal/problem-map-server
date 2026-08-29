package models_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

func (suite *ModelsSuite) TestWebhookBackoff() {
	tests := []struct {
		attempt int
		want    time.Duration
		ok      bool
	}{
		{attempt: 0},
		{attempt: 1, want: time.Minute, ok: true},
		{attempt: 2, want: 5 * time.Minute, ok: true},
		{attempt: 3, want: 30 * time.Minute, ok: true},
		{attempt: 4, want: 2 * time.Hour, ok: true},
		{attempt: 5, want: 12 * time.Hour, ok: true},
		{attempt: 6},
		{attempt: 100},
	}
	for _, tt := range tests {
		got, ok := models.WebhookBackoff(tt.attempt)
		suite.Equal(tt.ok, ok, "attempt %d", tt.attempt)
		suite.Equal(tt.want, got, "attempt %d", tt.attempt)
	}
	// The last retry happens on attempt MaxWebhookAttempts.
	_, ok := models.WebhookBackoff(models.MaxWebhookAttempts - 1)
	suite.True(ok)
	_, ok = models.WebhookBackoff(models.MaxWebhookAttempts)
	suite.False(ok)
}

func (suite *ModelsSuite) TestWebhookEventsMatch() {
	tests := []struct {
		name    string
		events  []string
		subject string
		want    bool
	}{
		{name: "Exact", events: []string{"mark.status_changed"}, subject: "mark.status_changed", want: true},
		{name: "OtherSubject", events: []string{"mark.status_changed"}, subject: "check.added"},
		{name: "Prefix", events: []string{"mark.*"}, subject: "mark.status_changed", want: true},
		{name: "PrefixOtherDomain", events: []string{"mark.*"}, subject: "task.assigned"},
		{name: "All", events: []string{"*"}, subject: "task.assigned", want: true},
		{name: "Empty", events: nil, subject: "task.assigned"},
		{name: "NoDotSubjectMatchesItsPrefixPattern", events: []string{"ping.*"}, subject: "ping", want: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.want, models.WebhookEventsMatch(tt.events, tt.subject))
			suite.Equal(tt.want, models.Webhook{Events: tt.events}.MatchesEvent(tt.subject))
		})
	}
}

func (suite *ModelsSuite) TestValidateWebhookEvents() {
	known := []string{"mark.status_changed", "task.assigned", "check.added"}
	tooMany := make([]string, models.MaxWebhookEvents+1)
	for i := range tooMany {
		tooMany[i] = "*"
	}

	tests := []struct {
		name    string
		events  []string
		wantErr bool
	}{
		{name: "Exact", events: []string{"mark.status_changed", "check.added"}},
		{name: "Prefix", events: []string{"mark.*"}},
		{name: "All", events: []string{"*"}},
		{name: "Empty", events: nil, wantErr: true},
		{name: "Unknown", events: []string{"mark.deleted"}, wantErr: true},
		{name: "UnknownPrefix", events: []string{"user.*"}, wantErr: true},
		{name: "TooMany", events: tooMany, wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := models.ValidateWebhookEvents(tt.events, known)
			if tt.wantErr {
				suite.ErrorIs(err, models.ErrInvalidWebhookEvents)
				return
			}
			suite.NoError(err)
		})
	}
}
