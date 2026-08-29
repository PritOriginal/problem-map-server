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
}

func NewChecks(log *slog.Logger, trManager trm.Manager, markStatusUpdater MarkStatusUpdater, repos ChecksRepositories) *Checks {
	return &Checks{
		log:               log,
		trManager:         trManager,
		repos:             repos,
		markStatusUpdater: markStatusUpdater,
	}
}

func (uc *Checks) AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error) {
	const op = "usecase.Checks.AddCheck"

	historyItem, err := uc.repos.Marks.GetLastMarkStatusHistoryItem(ctx, check.MarkID)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}
	check.MarkStatusHistoryItemId = historyItem.ID
	check.MarkStatusId = historyItem.NewMarkStatusID

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

		task, err := uc.repos.Tasks.GetTaskByUserIdAndMarkId(ctx, check.UserID, check.MarkID)
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
	log   *slog.Logger
	repos UpdaterRepositories
}

func NewUpdater(log *slog.Logger, repos UpdaterRepositories) *Updater {
	return &Updater{
		log:   log,
		repos: repos,
	}
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
		} else if score <= -3 {
			newMarkStatusId, err := u.reject(ctx, mark)
			if err != nil {
				return mapRepoErr(op, err)
			}
			u.log.Debug("change mark status", slog.Int("old", int(mark.MarkStatusID)), slog.Int("new", int(newMarkStatusId)))
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

	return u.confirm(ctx, mark)
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

	return u.reject(ctx, mark)
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
