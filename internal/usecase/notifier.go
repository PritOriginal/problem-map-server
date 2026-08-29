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

// NotifierOrganizationsRepository resolves the members of an organization.
type NotifierOrganizationsRepository interface {
	GetMemberIDs(ctx context.Context, orgId int) ([]int, error)
}

// NotifierUsersRepository resolves the moderators (users.role in
// moderator, admin).
type NotifierUsersRepository interface {
	GetUserIDsByRole(ctx context.Context, roles ...models.Role) ([]int, error)
}

type NotifierRepositories struct {
	Marks         NotifierMarksRepository
	Organizations NotifierOrganizationsRepository
	// Users is needed by HandleMarkHidden (moderators are notified).
	Users NotifierUsersRepository
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
	models.InProgressStatus:   "В работе",
	models.DuplicateStatus:    "Дубликат",
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

// HandleMarkAssigned notifies every member of the organization the mark
// was assigned to.
func (uc *Notifier) HandleMarkAssigned(ctx context.Context, ev events.MarkAssigned) error {
	const op = "usecase.Notifier.HandleMarkAssigned"

	return uc.notifyMembers(ctx, op, ev.EventID, ev.OrganizationID, models.Notification{
		Type:   models.NotificationMarkAssigned,
		MarkID: null.IntFrom(int64(ev.MarkID)),
		Title:  "Новая метка в очереди",
		Body:   fmt.Sprintf("Метка #%d назначена вашей службе, срок до %s", ev.MarkID, ev.SLADueAt.Local().Format("02.01.2006 15:04")),
	})
}

// HandleMarkSLABreached notifies every member of the organization that the
// deadline of the mark has passed. The event id is deterministic, so the
// repeated events of the periodic check are ignored by the store.
func (uc *Notifier) HandleMarkSLABreached(ctx context.Context, ev events.MarkSLABreached) error {
	const op = "usecase.Notifier.HandleMarkSLABreached"

	return uc.notifyMembers(ctx, op, ev.EventID, ev.OrganizationID, models.Notification{
		Type:   models.NotificationMarkSLABreached,
		MarkID: null.IntFrom(int64(ev.MarkID)),
		Title:  "Просрочен срок по метке",
		Body:   fmt.Sprintf("Срок решения метки #%d истёк %s", ev.MarkID, ev.SLADueAt.Local().Format("02.01.2006 15:04")),
	})
}

// HandleMarkHidden notifies the author of the mark and every moderator
// (users with the moderator or admin role).
func (uc *Notifier) HandleMarkHidden(ctx context.Context, ev events.MarkHidden) error {
	const op = "usecase.Notifier.HandleMarkHidden"

	if uc.repos.Users == nil {
		return fmt.Errorf("%s: users repository is not configured", op)
	}

	authorID := ev.AuthorID
	if authorID == 0 {
		mark, err := uc.repos.Marks.GetMarkById(ctx, ev.MarkID)
		if err != nil {
			return mapRepoErr(op, err)
		}
		authorID = mark.UserID
	}

	reason := "скрыта модератором"
	if ev.ReportsCount > 0 {
		reason = fmt.Sprintf("скрыта автоматически: %d жалоб", ev.ReportsCount)
	}

	if _, _, err := uc.notifications.Create(ctx, models.Notification{
		UserID:  authorID,
		EventID: ev.EventID,
		Type:    models.NotificationMarkHidden,
		MarkID:  null.IntFrom(int64(ev.MarkID)),
		Title:   "Ваша метка скрыта",
		Body:    fmt.Sprintf("Метка #%d %s и не показывается другим пользователям", ev.MarkID, reason),
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	moderators, err := uc.repos.Users.GetUserIDsByRole(ctx, models.RoleModerator, models.RoleAdmin)
	if err != nil {
		return mapRepoErr(op, err)
	}
	for _, userID := range moderators {
		if userID == authorID {
			continue
		}
		if _, _, err := uc.notifications.Create(ctx, models.Notification{
			UserID:  userID,
			EventID: ev.EventID,
			Type:    models.NotificationMarkHidden,
			MarkID:  null.IntFrom(int64(ev.MarkID)),
			Title:   "Метка скрыта",
			Body:    fmt.Sprintf("Метка #%d %s", ev.MarkID, reason),
		}); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

// HandleMarkMerged notifies the author of the merged mark and the users
// who followed it (their subscriptions now point at the target mark).
func (uc *Notifier) HandleMarkMerged(ctx context.Context, ev events.MarkMerged) error {
	const op = "usecase.Notifier.HandleMarkMerged"

	authorID := ev.AuthorID
	if authorID == 0 {
		mark, err := uc.repos.Marks.GetMarkById(ctx, ev.MarkID)
		if err != nil {
			return mapRepoErr(op, err)
		}
		authorID = mark.UserID
	}

	body := fmt.Sprintf("Метка #%d объединена с меткой #%d как дубликат", ev.MarkID, ev.TargetMarkID)
	if _, _, err := uc.notifications.Create(ctx, models.Notification{
		UserID:  authorID,
		EventID: ev.EventID,
		Type:    models.NotificationMarkMerged,
		MarkID:  null.IntFrom(int64(ev.TargetMarkID)),
		Title:   "Ваша метка объединена с другой",
		Body:    body,
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, userID := range ev.FollowerIDs {
		if userID == authorID {
			continue
		}
		if _, _, err := uc.notifications.Create(ctx, models.Notification{
			UserID:  userID,
			EventID: ev.EventID,
			Type:    models.NotificationMarkMerged,
			MarkID:  null.IntFrom(int64(ev.TargetMarkID)),
			Title:   "Метка объединена с другой",
			Body:    body + "; ваша подписка перенесена",
		}); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

// notifyMembers creates the notification for every member of the
// organization; the event id makes each of them idempotent.
func (uc *Notifier) notifyMembers(ctx context.Context, op, eventID string, orgID int, n models.Notification) error {
	if uc.repos.Organizations == nil {
		return fmt.Errorf("%s: organizations repository is not configured", op)
	}
	members, err := uc.repos.Organizations.GetMemberIDs(ctx, orgID)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if len(members) == 0 {
		uc.log.Debug("organization has no members, no notification", slog.String("op", op), slog.Int("organization_id", orgID))
		return nil
	}

	n.EventID = eventID
	for _, userID := range members {
		n.UserID = userID
		if _, _, err := uc.notifications.Create(ctx, n); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}
	return nil
}
