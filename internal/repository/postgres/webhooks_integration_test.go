//go:build integration

package postgres_test

import (
	"encoding/json"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/google/uuid"
	"github.com/guregu/null/v6"
)

func (s *PostgresSuite) newWebhook(owner int, events ...string) models.Webhook {
	return models.Webhook{
		OwnerUserID: owner,
		URL:         "https://example.org/hook",
		Secret:      "s3cr3t",
		Events:      events,
		Active:      true,
	}
}

func (s *PostgresSuite) addWebhook(owner int, events ...string) models.Webhook {
	id, err := s.webhooks.AddWebhook(s.ctx, s.newWebhook(owner, events...))
	s.Require().NoError(err)
	w, err := s.webhooks.GetWebhookById(s.ctx, int(id))
	s.Require().NoError(err)
	return w
}

func (s *PostgresSuite) newDelivery(webhookID int, subject string) models.WebhookDelivery {
	return models.WebhookDelivery{
		WebhookID: webhookID,
		EventID:   uuid.NewString(),
		Subject:   subject,
		Payload:   json.RawMessage(`{"event_id":"x","subject":"` + subject + `","data":{"mark_id":1}}`),
	}
}

func (s *PostgresSuite) TestWebhooks_CRUD() {
	w := s.addWebhook(fxUserBob, "mark.*", "check.added")
	s.Positive(w.ID)
	s.Equal(fxUserBob, w.OwnerUserID)
	s.Equal([]string{"mark.*", "check.added"}, w.Events)
	s.Equal("s3cr3t", w.Secret)
	s.True(w.Active)
	s.WithinDuration(time.Now(), w.CreatedAt, time.Minute)

	s.Run("list by owner", func() {
		second := s.addWebhook(fxUserBob, "*")
		s.addWebhook(fxUserAlice, "*")

		hooks, err := s.webhooks.GetWebhooksByOwner(s.ctx, fxUserBob)
		s.Require().NoError(err)
		s.Equal([]int{w.ID, second.ID}, ids(hooks, func(h models.Webhook) int { return h.ID }))

		none, err := s.webhooks.GetWebhooksByOwner(s.ctx, 404)
		s.Require().NoError(err)
		s.Empty(none)
	})

	s.Run("update events and active", func() {
		active := false
		s.Require().NoError(s.webhooks.UpdateWebhook(s.ctx, w.ID, models.WebhookUpdate{Active: &active}))
		got, err := s.webhooks.GetWebhookById(s.ctx, w.ID)
		s.Require().NoError(err)
		s.False(got.Active)
		s.Equal([]string{"mark.*", "check.added"}, got.Events, "events untouched")

		s.Require().NoError(s.webhooks.UpdateWebhook(s.ctx, w.ID, models.WebhookUpdate{Events: []string{"task.assigned"}}))
		got, err = s.webhooks.GetWebhookById(s.ctx, w.ID)
		s.Require().NoError(err)
		s.False(got.Active, "active untouched")
		s.Equal([]string{"task.assigned"}, got.Events)

		s.ErrorIs(s.webhooks.UpdateWebhook(s.ctx, 404, models.WebhookUpdate{Active: &active}), repository.ErrNotFound)
	})

	s.Run("unknown owner violates the foreign key", func() {
		_, err := s.webhooks.AddWebhook(s.ctx, s.newWebhook(404, "*"))
		s.ErrorIs(err, repository.ErrInvalidReference)
	})

	s.Run("delete cascades to deliveries", func() {
		_, created, err := s.webhooks.AddDelivery(s.ctx, s.newDelivery(w.ID, "task.assigned"))
		s.Require().NoError(err)
		s.True(created)

		s.Require().NoError(s.webhooks.DeleteWebhook(s.ctx, w.ID))
		_, err = s.webhooks.GetWebhookById(s.ctx, w.ID)
		s.ErrorIs(err, repository.ErrNotFound)
		s.Zero(s.countRows("webhook_deliveries", "webhook_id = $1", w.ID))
		s.ErrorIs(s.webhooks.DeleteWebhook(s.ctx, w.ID), repository.ErrNotFound)
	})
}

func (s *PostgresSuite) TestWebhooks_GetActiveWebhooksForEvent() {
	exact := s.addWebhook(fxUserBob, "mark.status_changed")
	prefix := s.addWebhook(fxUserBob, "mark.*")
	all := s.addWebhook(fxUserAlice, "*")
	other := s.addWebhook(fxUserBob, "task.assigned", "check.*")
	inactive := s.addWebhook(fxUserBob, "*")
	off := false
	s.Require().NoError(s.webhooks.UpdateWebhook(s.ctx, inactive.ID, models.WebhookUpdate{Active: &off}))

	tests := []struct {
		subject string
		wantIDs []int
	}{
		{subject: "mark.status_changed", wantIDs: []int{exact.ID, prefix.ID, all.ID}},
		{subject: "mark.deleted", wantIDs: []int{prefix.ID, all.ID}},
		{subject: "check.added", wantIDs: []int{all.ID, other.ID}},
		{subject: "task.assigned", wantIDs: []int{all.ID, other.ID}},
		{subject: "user.registered", wantIDs: []int{all.ID}},
	}
	for _, tt := range tests {
		s.Run(tt.subject, func() {
			hooks, err := s.webhooks.GetActiveWebhooksForEvent(s.ctx, tt.subject)
			s.Require().NoError(err)
			s.Equal(tt.wantIDs, ids(hooks, func(h models.Webhook) int { return h.ID }))
			for _, h := range hooks {
				s.True(h.MatchesEvent(tt.subject), "SQL and Go matching agree for %d", h.ID)
			}
		})
	}
}

func (s *PostgresSuite) TestWebhooks_Deliveries() {
	w := s.addWebhook(fxUserBob, "*")

	d := s.newDelivery(w.ID, "mark.status_changed")
	d.NextAttemptAt = null.TimeFrom(time.Now().Add(time.Minute))
	stored, created, err := s.webhooks.AddDelivery(s.ctx, d)
	s.Require().NoError(err)
	s.True(created)
	s.Positive(stored.ID)
	s.Equal(d.EventID, stored.EventID)
	s.Equal(0, stored.Attempt)
	s.JSONEq(string(d.Payload), string(stored.Payload))
	s.True(stored.NextAttemptAt.Valid)
	s.False(stored.Delivered())

	s.Run("same event to the same webhook is not duplicated", func() {
		again, created, err := s.webhooks.AddDelivery(s.ctx, d)
		s.Require().NoError(err)
		s.False(created)
		s.Equal(stored.ID, again.ID)
		s.Equal(1, s.countRows("webhook_deliveries", "webhook_id = $1", w.ID))
	})

	s.Run("same event to another webhook is stored", func() {
		w2 := s.addWebhook(fxUserAlice, "*")
		d2 := d
		d2.WebhookID = w2.ID
		_, created, err := s.webhooks.AddDelivery(s.ctx, d2)
		s.Require().NoError(err)
		s.True(created)
	})

	s.Run("unknown webhook violates the foreign key", func() {
		_, _, err := s.webhooks.AddDelivery(s.ctx, s.newDelivery(404, "x"))
		s.ErrorIs(err, repository.ErrInvalidReference)
	})

	s.Run("record failed and successful attempts", func() {
		now := time.Now().UTC().Truncate(time.Microsecond)
		s.Require().NoError(s.webhooks.RecordAttempt(s.ctx, stored.ID, models.WebhookAttemptResult{
			Attempt:       1,
			StatusCode:    null.IntFrom(503),
			Error:         null.StringFrom("unexpected status 503"),
			NextAttemptAt: null.TimeFrom(now.Add(5 * time.Minute)),
		}))
		got, err := s.webhooks.GetDeliveryById(s.ctx, stored.ID)
		s.Require().NoError(err)
		s.Equal(1, got.Attempt)
		s.Equal(int64(503), got.StatusCode.ValueOrZero())
		s.Equal("unexpected status 503", got.Error.ValueOrZero())
		s.False(got.DeliveredAt.Valid)
		s.WithinDuration(now.Add(5*time.Minute), got.NextAttemptAt.Time, time.Millisecond)

		s.Require().NoError(s.webhooks.RecordAttempt(s.ctx, stored.ID, models.WebhookAttemptResult{
			Attempt:     2,
			StatusCode:  null.IntFrom(200),
			DeliveredAt: null.TimeFrom(now),
		}))
		got, err = s.webhooks.GetDeliveryById(s.ctx, stored.ID)
		s.Require().NoError(err)
		s.Equal(2, got.Attempt)
		s.True(got.Delivered())
		s.False(got.Error.Valid, "error cleared on success")
		s.False(got.NextAttemptAt.Valid)

		s.ErrorIs(s.webhooks.RecordAttempt(s.ctx, 404, models.WebhookAttemptResult{Attempt: 1}), repository.ErrNotFound)
	})

	s.Run("list newest first with total", func() {
		for range 3 {
			_, _, err := s.webhooks.AddDelivery(s.ctx, s.newDelivery(w.ID, "check.added"))
			s.Require().NoError(err)
		}
		page, err := s.webhooks.GetDeliveriesByWebhookId(s.ctx, w.ID, models.Pagination{Limit: 2})
		s.Require().NoError(err)
		s.Equal(4, page.Total)
		s.Require().Len(page.Items, 2)
		s.Greater(page.Items[0].ID, page.Items[1].ID)
		s.Equal("check.added", page.Items[0].Subject)

		last, err := s.webhooks.GetDeliveriesByWebhookId(s.ctx, w.ID, models.Pagination{Limit: 2, Offset: 3})
		s.Require().NoError(err)
		s.Equal(4, last.Total)
		s.Require().Len(last.Items, 1)
		s.Equal(stored.ID, last.Items[0].ID)
	})
}

func (s *PostgresSuite) TestWebhooks_ClaimDueDeliveries() {
	active := s.addWebhook(fxUserBob, "*")
	disabled := s.addWebhook(fxUserAlice, "*")
	off := false
	s.Require().NoError(s.webhooks.UpdateWebhook(s.ctx, disabled.ID, models.WebhookUpdate{Active: &off}))

	now := time.Now().UTC().Truncate(time.Microsecond)
	add := func(webhookID int, next null.Time) models.WebhookDelivery {
		d := s.newDelivery(webhookID, "mark.status_changed")
		d.NextAttemptAt = next
		stored, _, err := s.webhooks.AddDelivery(s.ctx, d)
		s.Require().NoError(err)
		return stored
	}
	dueOld := add(active.ID, null.TimeFrom(now.Add(-time.Hour)))
	dueNew := add(active.ID, null.TimeFrom(now.Add(-time.Minute)))
	future := add(active.ID, null.TimeFrom(now.Add(time.Hour)))
	settled := add(active.ID, null.Time{})
	ofDisabled := add(disabled.ID, null.TimeFrom(now.Add(-time.Hour)))

	s.Run("oldest due first, limited, with the webhook", func() {
		pending, err := s.webhooks.ClaimDueDeliveries(s.ctx, now, 2*time.Minute, 1)
		s.Require().NoError(err)
		s.Require().Len(pending, 1)
		s.Equal(dueOld.ID, pending[0].Delivery.ID)
		s.Equal(active.ID, pending[0].Webhook.ID)
		s.Equal("s3cr3t", pending[0].Webhook.Secret)
		s.Equal([]string{"*"}, pending[0].Webhook.Events)
		s.JSONEq(string(dueOld.Payload), string(pending[0].Delivery.Payload))
		// The claim moved next_attempt_at into the future (lease).
		s.WithinDuration(now.Add(2*time.Minute), pending[0].Delivery.NextAttemptAt.Time, time.Millisecond)
	})

	s.Run("claimed rows are not returned again until the lease expires", func() {
		pending, err := s.webhooks.ClaimDueDeliveries(s.ctx, now, 2*time.Minute, 10)
		s.Require().NoError(err)
		s.Equal([]int{int(dueNew.ID)}, ids(pending, func(p models.PendingWebhookDelivery) int { return int(p.Delivery.ID) }))

		pending, err = s.webhooks.ClaimDueDeliveries(s.ctx, now, 2*time.Minute, 10)
		s.Require().NoError(err)
		s.Empty(pending)

		// After the lease both are due again; future, settled and the
		// disabled webhook's delivery stay untouched.
		pending, err = s.webhooks.ClaimDueDeliveries(s.ctx, now.Add(3*time.Minute), time.Minute, 10)
		s.Require().NoError(err)
		s.Equal([]int{int(dueOld.ID), int(dueNew.ID)}, ids(pending, func(p models.PendingWebhookDelivery) int { return int(p.Delivery.ID) }))
	})

	got, err := s.webhooks.GetDeliveryById(s.ctx, future.ID)
	s.Require().NoError(err)
	s.WithinDuration(now.Add(time.Hour), got.NextAttemptAt.Time, time.Millisecond)
	got, err = s.webhooks.GetDeliveryById(s.ctx, settled.ID)
	s.Require().NoError(err)
	s.False(got.NextAttemptAt.Valid)
	got, err = s.webhooks.GetDeliveryById(s.ctx, ofDisabled.ID)
	s.Require().NoError(err)
	s.WithinDuration(now.Add(-time.Hour), got.NextAttemptAt.Time, time.Millisecond)
}
