package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

type ChecksRepository interface {
	AddCheck(ctx context.Context, check models.Check) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	GetChecksByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Check], error)
	GetChecksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Check], error)
	GetChecksByMarkHistoryId(ctx context.Context, markHistoryId int) ([]models.Check, error)
	GetChecksByUserIdAndMarkId(ctx context.Context, userId int, markId int) ([]models.Check, error)
	GetChecksByUserIdAndMarkIdSince(ctx context.Context, userId int, markId int, dateTime time.Time) ([]models.Check, error)
	GetUserMarkCheck(ctx context.Context, userId int, markStatusHistoryId int) (models.Check, error)
}

type MarkStatusUpdater interface {
	Update(ctx context.Context, markId int) error
}

type ChecksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Tasks  TasksRepository
	Photos PhotosRepository
}

type Checks struct {
	log               *slog.Logger
	trManager         trm.Manager
	repos             ChecksRepositories
	markStatusUpdater MarkStatusUpdater
	events            events.Publisher
}

func NewChecks(log *slog.Logger, trManager trm.Manager, markStatusUpdater MarkStatusUpdater, repos ChecksRepositories) *Checks {
	return &Checks{
		log:               log,
		trManager:         trManager,
		repos:             repos,
		markStatusUpdater: markStatusUpdater,
		events:            events.NoopPublisher{},
	}
}

// WithEvents sets the publisher of domain events (check.added and the
// mark.status_changed produced by the status updater). Without it events
// are dropped.
func (uc *Checks) WithEvents(p events.Publisher) *Checks {
	if p != nil {
		uc.events = p
	}
	return uc
}

func (uc *Checks) AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error) {
	const op = "usecase.Checks.AddCheck"

	historyItem, err := uc.repos.Marks.GetLastMarkStatusHistoryItem(ctx, check.MarkID)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}
	check.MarkStatusHistoryItemId = historyItem.ID
	check.MarkStatusId = historyItem.NewMarkStatusID

	// Events raised inside the transaction (a status change made by the
	// updater) are queued and published only after a successful commit, so
	// a rolled back change never produces a notification.
	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)

	var checkId int64
	err = uc.trManager.Do(ctx, func(ctx context.Context) error {
		hasPossibilityAdd, err := uc.checkPossibilityAddCheck(ctx, check.UserID, check.MarkStatusHistoryItemId)
		if err != nil {
			return err
		}
		if !hasPossibilityAdd {
			return ErrConflict
		}

		checkId, err = uc.repos.Checks.AddCheck(ctx, check)
		if err != nil {
			return err
		}

		if err := uc.repos.Photos.AddPhotos(ctx, check.MarkID, int(checkId), photos); err != nil {
			return err
		}

		if err := uc.markStatusUpdater.Update(ctx, check.MarkID); err != nil {
			return err
		}

		// Only the issued task is closed: a completed or overdue task from an
		// earlier cycle must not shadow the current one.
		task, err := uc.repos.Tasks.GetTaskByUserIdAndMarkId(ctx, check.UserID, check.MarkID, models.UnfulfilledStatus)
		switch {
		case err == nil:
			return uc.repos.Tasks.UpdateTaskStatus(ctx, task.ID, models.CompletedStatus)
		case errors.Is(err, repository.ErrNotFound):
			return nil
		default:
			return err
		}
	})
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	events.PublishEvent(ctx, uc.log, uc.events, events.NewCheckAdded(int(checkId), check.MarkID, check.UserID))
	pending.Flush(ctx, uc.log, uc.events)

	return checkId, nil
}

func (uc *Checks) checkPossibilityAddCheck(ctx context.Context, userId int, historyId int) (bool, error) {
	const op = "usecase.Checks.checkPossibilityAddCheck"

	_, err := uc.repos.Checks.GetUserMarkCheck(ctx, userId, historyId)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			return true, nil
		default:
			return false, mapRepoErr(op, err)
		}
	}

	return false, nil
}

func (uc *Checks) GetCheckById(ctx context.Context, id int) (models.Check, error) {
	const op = "usecase.Checks.GetCheckById"

	check, err := uc.repos.Checks.GetCheckById(ctx, id)
	if err != nil {
		return check, mapRepoErr(op, err)
	}

	check.Photos, err = uc.repos.Photos.GetPhotosByCheckId(ctx, check.MarkID, check.ID)
	if err != nil {
		return check, mapRepoErr(op, err)
	}

	return check, nil
}

// ListChecksByMarkId returns a page of the mark's checks (with photos) and the total count.
func (uc *Checks) ListChecksByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Check], error) {
	const op = "usecase.Checks.ListChecksByMarkId"

	if err := p.Validate(); err != nil {
		return models.Page[models.Check]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Checks.GetChecksByMarkId(ctx, markId, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	photosMap, err := uc.repos.Photos.GetPhotosByMarkId(ctx, markId)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	for i := range page.Items {
		page.Items[i].Photos = photosMap[markId][page.Items[i].ID]
	}

	return page, nil
}

func (uc *Checks) GetGroupedChecksByMarkStatusHistoryId(ctx context.Context, markId int) ([]models.GroupedChecksByMarkStatusHistoryId, error) {
	const op = "usecase.Checks.GetGroupedChecksByMarkStatusHistoryId"

	page, err := uc.repos.Checks.GetChecksByMarkId(ctx, markId, models.Pagination{})
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	checks := page.Items

	photosMap, err := uc.repos.Photos.GetPhotosByMarkId(ctx, markId)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	for i := range len(checks) {
		checks[i].Photos = photosMap[markId][checks[i].ID]
	}

	groupedChecks := uc.groupChecks(checks)

	return groupedChecks, nil
}

func (uc *Checks) groupChecks(checks []models.Check) []models.GroupedChecksByMarkStatusHistoryId {
	groupedChecksMap := make(map[int][]models.Check, 0)
	for _, check := range checks {
		historyItemId := check.MarkStatusHistoryItemId
		groupedChecksMap[historyItemId] = append(groupedChecksMap[historyItemId], check)
	}

	keys := slices.Collect(maps.Keys(groupedChecksMap))
	slices.Sort(keys)

	groupedChecks := make([]models.GroupedChecksByMarkStatusHistoryId, 0, len(groupedChecksMap))
	for _, k := range keys {
		groupedChecks = append(groupedChecks, models.GroupedChecksByMarkStatusHistoryId{
			MarkStatusHistoryId: k,
			Сhecks:              groupedChecksMap[k],
		})
	}

	return groupedChecks
}

// ListChecksByUserId returns a page of the user's checks (with photos) and the total count.
func (uc *Checks) ListChecksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Check], error) {
	const op = "usecase.Checks.ListChecksByUserId"

	if err := p.Validate(); err != nil {
		return models.Page[models.Check]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Checks.GetChecksByUserId(ctx, userId, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	// One storage listing per distinct mark instead of one per check.
	photosByMark := map[int]map[int][]string{}
	for i := range page.Items {
		markId := page.Items[i].MarkID
		if _, ok := photosByMark[markId]; !ok {
			photosMap, err := uc.repos.Photos.GetPhotosByMarkId(ctx, markId)
			if err != nil {
				return page, mapRepoErr(op, err)
			}
			photosByMark[markId] = photosMap[markId]
		}
		page.Items[i].Photos = photosByMark[markId][page.Items[i].ID]
	}

	return page, nil
}

type UpdaterRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
}
type Updater struct {
	log    *slog.Logger
	repos  UpdaterRepositories
	events events.Publisher
}

func NewUpdater(log *slog.Logger, repos UpdaterRepositories) *Updater {
	return &Updater{
		log:    log,
		repos:  repos,
		events: events.NoopPublisher{},
	}
}

// WithEvents sets the publisher of mark.status_changed events. Without it
// events are dropped.
func (u *Updater) WithEvents(p events.Publisher) *Updater {
	if p != nil {
		u.events = p
	}
	return u
}

// statusChanged reports a status transition of mark. Inside a transaction
// (see Checks.AddCheck) the event is queued on the context and published
// after the commit; otherwise it is published right away.
func (u *Updater) statusChanged(ctx context.Context, mark models.Mark, newStatus models.MarkStatusType) {
	ev := events.NewMarkStatusChanged(mark.ID, mark.MarkStatusID, newStatus, mark.UserID)
	if events.Collect(ctx, ev) {
		return
	}
	events.PublishEvent(ctx, u.log, u.events, ev)
}

func (u *Updater) Update(ctx context.Context, markId int) error {
	const op = "usecase.Updater.Update"

	mark, err := u.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return mapRepoErr(op, err)
	}

	historyItem, err := u.repos.Marks.GetLastMarkStatusHistoryItem(ctx, markId)
	if err != nil {
		return mapRepoErr(op, err)
	}

	if mark.MarkStatusID == models.UnconfirmedStatus || mark.MarkStatusID == models.UnderReviewStatus {
		checks, err := u.repos.Checks.GetChecksByMarkHistoryId(ctx, historyItem.ID)
		if err != nil {
			return mapRepoErr(op, err)
		}

		score := 0
		for _, check := range checks {
			if check.Result {
				score++
			} else {
				score--
			}
		}

		u.log.Debug("score", slog.Int("val", score))

		if score >= 3 {
			newMarkStatusId, err := u.confirm(ctx, mark)
			if err != nil {
				return mapRepoErr(op, err)
			}
			u.log.Debug("change mark status", slog.Int("old", int(mark.MarkStatusID)), slog.Int("new", int(newMarkStatusId)))
			u.statusChanged(ctx, mark, newMarkStatusId)
		} else if score <= -3 {
			newMarkStatusId, err := u.reject(ctx, mark)
			if err != nil {
				return mapRepoErr(op, err)
			}
			u.log.Debug("change mark status", slog.Int("old", int(mark.MarkStatusID)), slog.Int("new", int(newMarkStatusId)))
			u.statusChanged(ctx, mark, newMarkStatusId)
		}
	}
	return nil
}

func (u *Updater) Confirm(ctx context.Context, markId int) (models.MarkStatusType, error) {
	const op = "usecase.Map.Confirm"

	mark, err := u.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	newStatus, err := u.confirm(ctx, mark)
	if err != nil {
		return 0, err
	}
	u.statusChanged(ctx, mark, newStatus)

	return newStatus, nil
}

func (u *Updater) confirm(ctx context.Context, mark models.Mark) (models.MarkStatusType, error) {
	const op = "usecase.Map.confirm"

	var newStatus models.MarkStatusType

	switch mark.MarkStatusID {
	case models.UnconfirmedStatus:
		newStatus = models.ConfirmedStatus
	case models.ConfirmedStatus, models.RediscoveredStatus:
		newStatus = models.UnderReviewStatus
	case models.UnderReviewStatus:
		newStatus = models.ClosedStatus
	default:
		return 0, ErrConflict
	}

	if err := u.repos.Marks.UpdateMarkStatus(ctx, mark.ID, newStatus); err != nil {
		return 0, mapRepoErr(op, err)
	}

	return newStatus, nil

}

func (u *Updater) Reject(ctx context.Context, markId int) (models.MarkStatusType, error) {
	const op = "usecase.Map.Reject"

	mark, err := u.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	newStatus, err := u.reject(ctx, mark)
	if err != nil {
		return 0, err
	}
	u.statusChanged(ctx, mark, newStatus)

	return newStatus, nil
}

func (u *Updater) reject(ctx context.Context, mark models.Mark) (models.MarkStatusType, error) {
	const op = "usecase.Map.reject"

	var newStatus models.MarkStatusType

	switch mark.MarkStatusID {
	case models.UnconfirmedStatus, models.ConfirmedStatus:
		newStatus = models.RefutedStatus
	case models.RediscoveredStatus:
		newStatus = models.ClosedStatus
	case models.UnderReviewStatus:
		newStatus = models.RediscoveredStatus
	default:
		return 0, ErrConflict
	}

	if err := u.repos.Marks.UpdateMarkStatus(ctx, mark.ID, newStatus); err != nil {
		return 0, mapRepoErr(op, err)
	}

	return newStatus, nil
}
