package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/guregu/null/v6"
)

// NotificationCreator is the part of Notifications the event consumer needs.
type NotificationCreator interface {
	Create(ctx context.Context, n models.Notification) (id int64, created bool, err error)
}

// NotifierMarksRepository resolves the mark an event refers to.
type NotifierMarksRepository interface {
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
}

type NotifierRepositories struct {
	Marks NotifierMarksRepository
}

// Notifier turns domain events into notifications for their addressees.
// Every handler is idempotent: the event id is stored with the
// notification, so a redelivered event is a no-op.
type Notifier struct {
	log           *slog.Logger
	notifications NotificationCreator
	repos         NotifierRepositories
}

func NewNotifier(log *slog.Logger, notifications NotificationCreator, repos NotifierRepositories) *Notifier {
	return &Notifier{
		log:           log,
		notifications: notifications,
		repos:         repos,
	}
}

// markStatusNames are the human-readable names of mark statuses used in
// notification texts (they mirror the mark_statuses table).
var markStatusNames = map[models.MarkStatusType]string{
	models.UnconfirmedStatus:  "Не подтверждена",
	models.ConfirmedStatus:    "Подтверждена",
	models.UnderReviewStatus:  "На проверке",
	models.RediscoveredStatus: "Обнаружена повторно",
	models.ClosedStatus:       "Закрыта",
	models.RefutedStatus:      "Опровергнута",
}

func markStatusName(s models.MarkStatusType) string {
	if name, ok := markStatusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("статус %d", int(s))
}

// HandleMarkStatusChanged notifies the author of the mark.
func (uc *Notifier) HandleMarkStatusChanged(ctx context.Context, ev events.MarkStatusChanged) error {
	const op = "usecase.Notifier.HandleMarkStatusChanged"

	authorID := ev.AuthorID
	if authorID == 0 {
		mark, err := uc.repos.Marks.GetMarkById(ctx, ev.MarkID)
		if err != nil {
			return mapRepoErr(op, err)
		}
		authorID = mark.UserID
	}

	_, _, err := uc.notifications.Create(ctx, models.Notification{
		UserID:  authorID,
		EventID: ev.EventID,
		Type:    models.NotificationMarkStatusChanged,
		MarkID:  null.IntFrom(int64(ev.MarkID)),
		Title:   "Статус вашей метки изменён",
		Body:    fmt.Sprintf("Метка #%d: %s → %s", ev.MarkID, markStatusName(ev.OldStatus), markStatusName(ev.NewStatus)),
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// HandleTaskAssigned notifies the assignee.
func (uc *Notifier) HandleTaskAssigned(ctx context.Context, ev events.TaskAssigned) error {
	const op = "usecase.Notifier.HandleTaskAssigned"

	body := fmt.Sprintf("Вам назначена проверка метки #%d", ev.MarkID)
	if ev.DueAt != nil {
		body += " до " + ev.DueAt.Format("02.01.2006 15:04")
	}

	_, _, err := uc.notifications.Create(ctx, models.Notification{
		UserID:  ev.UserID,
		EventID: ev.EventID,
		Type:    models.NotificationTaskAssigned,
		MarkID:  null.IntFrom(int64(ev.MarkID)),
		TaskID:  null.IntFrom(int64(ev.TaskID)),
		Title:   "Новое задание",
		Body:    body,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// HandleCheckAdded notifies the author of the mark unless the check is
// their own.
func (uc *Notifier) HandleCheckAdded(ctx context.Context, ev events.CheckAdded) error {
	const op = "usecase.Notifier.HandleCheckAdded"

	mark, err := uc.repos.Marks.GetMarkById(ctx, ev.MarkID)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if mark.UserID == ev.UserID {
		uc.log.Debug("check by the mark author, no notification", slog.Int("mark_id", ev.MarkID))
		return nil
	}

	_, _, err = uc.notifications.Create(ctx, models.Notification{
		UserID:  mark.UserID,
		EventID: ev.EventID,
		Type:    models.NotificationCheckAdded,
		MarkID:  null.IntFrom(int64(ev.MarkID)),
		Title:   "Новая проверка вашей метки",
		Body:    fmt.Sprintf("Метка #%d получила новую проверку", ev.MarkID),
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
