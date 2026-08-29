//go:build integration

package postgres_test

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/google/uuid"
	"github.com/guregu/null/v6"
)

func (s *PostgresSuite) newNotification(userID int, eventID string) models.Notification {
	return models.Notification{
		UserID:  userID,
		EventID: eventID,
		Type:    models.NotificationTaskAssigned,
		MarkID:  null.IntFrom(fxMarkNear),
		TaskID:  null.IntFrom(fxTaskAliceMark1),
		Title:   "Новое задание",
		Body:    "Проверьте метку",
	}
}

func (s *PostgresSuite) TestNotifications_AddNotification() {
	eventID := uuid.NewString()

	id, created, err := s.notifications.AddNotification(s.ctx, s.newNotification(fxUserAlice, eventID))
	s.Require().NoError(err)
	s.True(created)
	s.Positive(id)

	s.Run("duplicate event for the same user is skipped", func() {
		dupID, dupCreated, err := s.notifications.AddNotification(s.ctx, s.newNotification(fxUserAlice, eventID))
		s.Require().NoError(err)
		s.False(dupCreated)
		s.Zero(dupID)
		s.Equal(1, s.countRows("notifications", "event_id = $1", eventID))
	})

	s.Run("same event for another user is stored", func() {
		_, created, err := s.notifications.AddNotification(s.ctx, s.newNotification(fxUserBob, eventID))
		s.Require().NoError(err)
		s.True(created)
		s.Equal(2, s.countRows("notifications", "event_id = $1", eventID))
	})

	s.Run("unknown user violates the foreign key", func() {
		_, _, err := s.notifications.AddNotification(s.ctx, s.newNotification(404, uuid.NewString()))
		s.Error(err)
	})
}

func (s *PostgresSuite) TestNotifications_ListAndRead() {
	var ids []int64
	for range 3 {
		id, _, err := s.notifications.AddNotification(s.ctx, s.newNotification(fxUserAlice, uuid.NewString()))
		s.Require().NoError(err)
		ids = append(ids, id)
	}
	_, _, err := s.notifications.AddNotification(s.ctx, s.newNotification(fxUserBob, uuid.NewString()))
	s.Require().NoError(err)

	page, err := s.notifications.GetNotificationsByUserId(s.ctx, fxUserAlice, models.GetNotificationsFilters{})
	s.Require().NoError(err)
	s.Equal(3, page.Total)
	s.Require().Len(page.Items, 3)
	// Newest first.
	s.Equal(int(ids[2]), page.Items[0].ID)
	s.Equal(int(ids[0]), page.Items[2].ID)
	s.False(page.Items[0].ReadAt.Valid)
	s.Equal(int64(fxMarkNear), page.Items[0].MarkID.ValueOrZero())
	s.Equal(int64(fxTaskAliceMark1), page.Items[0].TaskID.ValueOrZero())

	count, err := s.notifications.CountUnreadByUserId(s.ctx, fxUserAlice)
	s.Require().NoError(err)
	s.Equal(3, count)

	s.Run("mark one read", func() {
		s.Require().NoError(s.notifications.MarkRead(s.ctx, fxUserAlice, int(ids[0])))
		// Idempotent.
		s.Require().NoError(s.notifications.MarkRead(s.ctx, fxUserAlice, int(ids[0])))

		count, err := s.notifications.CountUnreadByUserId(s.ctx, fxUserAlice)
		s.Require().NoError(err)
		s.Equal(2, count)

		unread, err := s.notifications.GetNotificationsByUserId(s.ctx, fxUserAlice, models.GetNotificationsFilters{UnreadOnly: true})
		s.Require().NoError(err)
		s.Equal(2, unread.Total)
		s.NotContains(ids2ints(unread.Items), int(ids[0]))
	})

	s.Run("another user's notification is not found", func() {
		s.ErrorIs(s.notifications.MarkRead(s.ctx, fxUserBob, int(ids[1])), repository.ErrNotFound)
		s.ErrorIs(s.notifications.MarkRead(s.ctx, fxUserAlice, 404), repository.ErrNotFound)
	})

	s.Run("pagination", func() {
		page, err := s.notifications.GetNotificationsByUserId(s.ctx, fxUserAlice, models.GetNotificationsFilters{
			Pagination: models.Pagination{Limit: 2, Offset: 1},
		})
		s.Require().NoError(err)
		s.Equal(3, page.Total)
		s.Len(page.Items, 2)
	})

	s.Run("mark all read", func() {
		n, err := s.notifications.MarkAllRead(s.ctx, fxUserAlice)
		s.Require().NoError(err)
		s.Equal(int64(2), n)

		count, err := s.notifications.CountUnreadByUserId(s.ctx, fxUserAlice)
		s.Require().NoError(err)
		s.Zero(count)

		// Bob's notification is untouched.
		count, err = s.notifications.CountUnreadByUserId(s.ctx, fxUserBob)
		s.Require().NoError(err)
		s.Equal(1, count)
	})
}

func ids2ints(items []models.Notification) []int {
	return ids(items, func(n models.Notification) int { return n.ID })
}

func (s *PostgresSuite) TestNotifications_Devices() {
	device, err := s.notifications.UpsertDevice(s.ctx, models.UserDevice{UserID: fxUserAlice, Platform: models.PlatformAndroid, Token: "tok-1"})
	s.Require().NoError(err)
	s.Positive(device.ID)
	s.Equal(models.PlatformAndroid, device.Platform)
	s.False(device.CreatedAt.IsZero())

	s.Run("upsert keeps a single row per token", func() {
		again, err := s.notifications.UpsertDevice(s.ctx, models.UserDevice{UserID: fxUserAlice, Platform: models.PlatformIOS, Token: "tok-1"})
		s.Require().NoError(err)
		s.Equal(device.ID, again.ID)
		s.Equal(models.PlatformIOS, again.Platform)
		s.Equal(1, s.countRows("user_devices", "token = $1", "tok-1"))
	})

	s.Run("token moves to another user", func() {
		moved, err := s.notifications.UpsertDevice(s.ctx, models.UserDevice{UserID: fxUserBob, Platform: models.PlatformWeb, Token: "tok-1"})
		s.Require().NoError(err)
		s.Equal(fxUserBob, moved.UserID)

		alice, err := s.notifications.GetDevicesByUserId(s.ctx, fxUserAlice)
		s.Require().NoError(err)
		s.Empty(alice)

		bob, err := s.notifications.GetDevicesByUserId(s.ctx, fxUserBob)
		s.Require().NoError(err)
		s.Len(bob, 1)
	})

	s.Run("invalid platform is rejected by the check constraint", func() {
		_, err := s.notifications.UpsertDevice(s.ctx, models.UserDevice{UserID: fxUserAlice, Platform: "windows", Token: "tok-2"})
		s.Error(err)
	})

	s.Run("delete", func() {
		s.ErrorIs(s.notifications.DeleteDevice(s.ctx, fxUserAlice, "tok-1"), repository.ErrNotFound, "token belongs to Bob")
		s.Require().NoError(s.notifications.DeleteDevice(s.ctx, fxUserBob, "tok-1"))
		s.ErrorIs(s.notifications.DeleteDevice(s.ctx, fxUserBob, "tok-1"), repository.ErrNotFound)
	})
}
