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

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/guregu/null/v6"
)

// checksPerDayWindow is the rolling window of the daily check limit.
const checksPerDayWindow = 24 * time.Hour

type ChecksRepository interface {
	AddCheck(ctx context.Context, check models.Check) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	GetChecksByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Check], error)
	GetChecksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Check], error)
	GetChecksByMarkHistoryId(ctx context.Context, markHistoryId int) ([]models.Check, error)
	GetChecksByUserIdAndMarkId(ctx context.Context, userId int, markId int) ([]models.Check, error)
	GetChecksByUserIdAndMarkIdSince(ctx context.Context, userId int, markId int, dateTime time.Time) ([]models.Check, error)
	GetUserMarkCheck(ctx context.Context, userId int, markStatusHistoryId int) (models.Check, error)
	CountChecksByUserIdSince(ctx context.Context, userId int, since time.Time) (int, error)
}

type MarkStatusUpdater interface {
	Update(ctx context.Context, markId int) error
}

type ChecksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Tasks  TasksRepository
	Photos PhotosRepository
	Users  UsersRepository
}

type Checks struct {
	log               *slog.Logger
	cfg               config.RatingConfig
	trManager         trm.Manager
	repos             ChecksRepositories
	markStatusUpdater MarkStatusUpdater
}

func NewChecks(log *slog.Logger, cfg config.RatingConfig, trManager trm.Manager, markStatusUpdater MarkStatusUpdater, repos ChecksRepositories) *Checks {
	return &Checks{
		log:               log,
		cfg:               cfg,
		trManager:         trManager,
		repos:             repos,
		markStatusUpdater: markStatusUpdater,
	}
}

// AddCheck records a user's vote on the mark's current voting stage.
//
// Anti-fraud rules: the author may not check their own mark (ErrForbidden),
// a user may submit at most cfg.MaxChecksPerDay checks per rolling 24 hours
// (ErrTooManyRequests), and only one check per voting stage (ErrConflict).
func (uc *Checks) AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error) {
	const op = "usecase.Checks.AddCheck"

	mark, err := uc.repos.Marks.GetMarkById(ctx, check.MarkID)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}
	if mark.UserID == check.UserID {
		return 0, fmt.Errorf("%s: %w: own mark", op, ErrForbidden)
	}

	historyItem, err := uc.repos.Marks.GetLastMarkStatusHistoryItem(ctx, check.MarkID)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}
	check.MarkStatusHistoryItemId = historyItem.ID
	check.MarkStatusId = historyItem.NewMarkStatusID

	var checkId int64
	err = uc.trManager.Do(ctx, func(ctx context.Context) error {
		if err := uc.checkDailyLimit(ctx, check.UserID); err != nil {
			return err
		}

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
			if err := uc.repos.Tasks.UpdateTaskStatus(ctx, task.ID, models.CompletedStatus); err != nil {
				return err
			}
			_, err = uc.repos.Users.AddRatingEvent(ctx, models.RatingEvent{
				UserID:  check.UserID,
				Delta:   uc.cfg.TaskCompleted,
				Reason:  models.RatingReasonTaskCompleted,
				MarkID:  null.IntFrom(int64(check.MarkID)),
				CheckID: null.IntFrom(checkId),
			})
			return err
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

// checkDailyLimit returns ErrTooManyRequests when the user has already
// submitted cfg.MaxChecksPerDay checks in the last 24 hours.
func (uc *Checks) checkDailyLimit(ctx context.Context, userId int) error {
	n, err := uc.repos.Checks.CountChecksByUserIdSince(ctx, userId, time.Now().Add(-checksPerDayWindow))
	if err != nil {
		return err
	}
	if n >= uc.cfg.MaxChecksPerDay {
		return fmt.Errorf("%w: %d checks per day", ErrTooManyRequests, uc.cfg.MaxChecksPerDay)
	}
	return nil
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
	Users  UsersRepository
}

// Updater moves a mark through its status graph and awards rating for the
// resolved voting stage: checkers whose vote matched the outcome get
// cfg.CheckCorrect, the others cfg.CheckWrong; the author gets
// cfg.MarkConfirmed / cfg.MarkRefuted on the first decision about the mark
// (Unconfirmed -> Confirmed / Refuted).
type Updater struct {
	log       *slog.Logger
	cfg       config.RatingConfig
	trManager trm.Manager
	repos     UpdaterRepositories
}

func NewUpdater(log *slog.Logger, cfg config.RatingConfig, trManager trm.Manager, repos UpdaterRepositories) *Updater {
	return &Updater{
		log:       log,
		cfg:       cfg,
		trManager: trManager,
		repos:     repos,
	}
}

// Update resolves the current voting stage when the vote score reaches ±3.
// It is called inside the AddCheck transaction, so the status change and
// the rating events are committed together with the check.
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
			newMarkStatusId, err := u.confirm(ctx, mark, checks)
			if err != nil {
				return mapRepoErr(op, err)
			}
			u.log.Debug("change mark status", slog.Int("old", int(mark.MarkStatusID)), slog.Int("new", int(newMarkStatusId)))
		} else if score <= -3 {
			newMarkStatusId, err := u.reject(ctx, mark, checks)
			if err != nil {
				return mapRepoErr(op, err)
			}
			u.log.Debug("change mark status", slog.Int("old", int(mark.MarkStatusID)), slog.Int("new", int(newMarkStatusId)))
		}
	}
	return nil
}

// Confirm is the moderator's decision: the current stage resolves as
// confirmed regardless of the score. Runs in its own transaction.
func (u *Updater) Confirm(ctx context.Context, markId int) (models.MarkStatusType, error) {
	const op = "usecase.Map.Confirm"

	return u.decide(ctx, op, markId, u.confirm)
}

// Reject is the moderator's decision: the current stage resolves as
// rejected regardless of the score. Runs in its own transaction.
func (u *Updater) Reject(ctx context.Context, markId int) (models.MarkStatusType, error) {
	const op = "usecase.Map.Reject"

	return u.decide(ctx, op, markId, u.reject)
}

func (u *Updater) decide(ctx context.Context, op string, markId int,
	transition func(context.Context, models.Mark, []models.Check) (models.MarkStatusType, error),
) (models.MarkStatusType, error) {
	var newStatus models.MarkStatusType
	err := u.trManager.Do(ctx, func(ctx context.Context) error {
		mark, err := u.repos.Marks.GetMarkById(ctx, markId)
		if err != nil {
			return err
		}

		historyItem, err := u.repos.Marks.GetLastMarkStatusHistoryItem(ctx, markId)
		if err != nil {
			return err
		}

		checks, err := u.repos.Checks.GetChecksByMarkHistoryId(ctx, historyItem.ID)
		if err != nil {
			return err
		}

		newStatus, err = transition(ctx, mark, checks)
		return err
	})
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	return newStatus, nil
}

func (u *Updater) confirm(ctx context.Context, mark models.Mark, checks []models.Check) (models.MarkStatusType, error) {
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

	if err := u.transition(ctx, mark, newStatus, checks, true); err != nil {
		return 0, mapRepoErr(op, err)
	}

	return newStatus, nil
}

func (u *Updater) reject(ctx context.Context, mark models.Mark, checks []models.Check) (models.MarkStatusType, error) {
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

	if err := u.transition(ctx, mark, newStatus, checks, false); err != nil {
		return 0, mapRepoErr(op, err)
	}

	return newStatus, nil
}

// transition writes the new status and awards rating for the resolved
// stage. confirmed is the outcome the checks are compared against.
func (u *Updater) transition(ctx context.Context, mark models.Mark, newStatus models.MarkStatusType, checks []models.Check, confirmed bool) error {
	if err := u.repos.Marks.UpdateMarkStatus(ctx, mark.ID, newStatus); err != nil {
		return err
	}

	markId := null.IntFrom(int64(mark.ID))

	for _, check := range checks {
		event := models.RatingEvent{
			UserID:  check.UserID,
			Delta:   u.cfg.CheckWrong,
			Reason:  models.RatingReasonCheckWrong,
			MarkID:  markId,
			CheckID: null.IntFrom(int64(check.ID)),
		}
		if check.Result == confirmed {
			event.Delta = u.cfg.CheckCorrect
			event.Reason = models.RatingReasonCheckCorrect
		}
		if _, err := u.repos.Users.AddRatingEvent(ctx, event); err != nil {
			return err
		}
	}

	// The author is rated once, on the first decision about the mark.
	if mark.MarkStatusID != models.UnconfirmedStatus {
		return nil
	}
	event := models.RatingEvent{
		UserID: mark.UserID,
		Delta:  u.cfg.MarkRefuted,
		Reason: models.RatingReasonMarkRefuted,
		MarkID: markId,
	}
	if confirmed {
		event.Delta = u.cfg.MarkConfirmed
		event.Reason = models.RatingReasonMarkConfirmed
	}
	_, err := u.repos.Users.AddRatingEvent(ctx, event)
	return err
}
