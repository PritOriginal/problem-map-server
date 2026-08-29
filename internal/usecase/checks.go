package usecase

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
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

// MarkAssigner assigns a freshly confirmed mark to the responsible
// organization; it runs inside the updater's transaction.
type MarkAssigner interface {
	AssignConfirmed(ctx context.Context, mark models.Mark) error
}

// MembershipChecker reports whether a user belongs to an organization.
type MembershipChecker interface {
	IsMember(ctx context.Context, orgId, userId int) (bool, error)
}

type ChecksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Tasks  TasksRepository
	Photos PhotosRepository
	Users  UsersRepository
	// Organizations is optional: without it members of the assigned
	// organization are not barred from voting on their own marks.
	Organizations MembershipChecker
}

type Checks struct {
	log               *slog.Logger
	cfg               config.RatingConfig
	trManager         trm.Manager
	repos             ChecksRepositories
	markStatusUpdater MarkStatusUpdater
	events            events.Publisher
}

func NewChecks(log *slog.Logger, cfg config.RatingConfig, trManager trm.Manager, markStatusUpdater MarkStatusUpdater, repos ChecksRepositories) *Checks {
	return &Checks{
		log:               log,
		cfg:               cfg,
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

// AddCheck records a user's vote on the mark's current voting stage.
//
// Anti-fraud rules: the author may not check their own mark (ErrForbidden),
// neither may a member of the organization the mark is assigned to (the
// service reports its work through Organizations.Resolve and must not vote
// on, or earn rating for, the review of that work), a user may submit at
// most cfg.MaxChecksPerDay checks per rolling 24 hours (ErrTooManyRequests),
// and only one check per voting stage (ErrConflict).
func (uc *Checks) AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error) {
	const op = "usecase.Checks.AddCheck"

	// Events raised inside the transaction (a status change made by the
	// updater) are queued and published only after a successful commit, so
	// a rolled back change never produces a notification.
	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)

	var checkId int64
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		// Concurrent checks on one mark are serialised by the row lock, so
		// the stage is read, scored and resolved (markStatusUpdater) by one
		// transaction at a time and can never be rated twice.
		if err := uc.repos.Marks.LockMark(ctx, check.MarkID); err != nil {
			return err
		}

		mark, err := uc.repos.Marks.GetMarkById(ctx, check.MarkID)
		if err != nil {
			return err
		}
		if mark.UserID == check.UserID {
			return fmt.Errorf("%w: own mark", ErrForbidden)
		}
		if uc.repos.Organizations != nil && mark.OrganizationID.Valid {
			member, err := uc.repos.Organizations.IsMember(ctx, int(mark.OrganizationID.Int64), check.UserID)
			if err != nil {
				return err
			}
			if member {
				return fmt.Errorf("%w: member of the assigned organization", ErrForbidden)
			}
		}

		historyItem, err := uc.repos.Marks.GetLastMarkStatusHistoryItem(ctx, check.MarkID)
		if err != nil {
			return err
		}
		check.MarkStatusHistoryItemId = historyItem.ID
		check.MarkStatusId = historyItem.NewMarkStatusID

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

	events.PublishEvent(ctx, uc.log, uc.events, events.NewCheckAdded(int(checkId), check.MarkID, check.UserID))
	pending.Flush(ctx, uc.log, uc.events)

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
	events    events.Publisher
	assigner  MarkAssigner
}

func NewUpdater(log *slog.Logger, cfg config.RatingConfig, trManager trm.Manager, repos UpdaterRepositories) *Updater {
	return &Updater{
		log:       log,
		cfg:       cfg,
		trManager: trManager,
		repos:     repos,
		events:    events.NoopPublisher{},
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

// WithAssigner sets the assigner called when a mark becomes confirmed.
// Without it confirmed marks stay unassigned.
func (u *Updater) WithAssigner(a MarkAssigner) *Updater {
	if a != nil {
		u.assigner = a
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

// Update resolves the current voting stage when the vote score reaches ±3.
// It is called inside the AddCheck transaction, so the status change and
// the rating events are committed together with the check.
func (u *Updater) Update(ctx context.Context, markId int) error {
	const op = "usecase.Updater.Update"

	mark, checks, err := u.loadStage(ctx, markId)
	if err != nil {
		return mapRepoErr(op, err)
	}

	if mark.MarkStatusID == models.UnconfirmedStatus || mark.MarkStatusID == models.UnderReviewStatus {
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
			u.statusChanged(ctx, mark, newMarkStatusId)
		} else if score <= -3 {
			newMarkStatusId, err := u.reject(ctx, mark, checks)
			if err != nil {
				return mapRepoErr(op, err)
			}
			u.log.Debug("change mark status", slog.Int("old", int(mark.MarkStatusID)), slog.Int("new", int(newMarkStatusId)))
			u.statusChanged(ctx, mark, newMarkStatusId)
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
	// Events raised inside the transaction (the assignment of a confirmed
	// mark) are published only after the commit.
	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)

	var newStatus models.MarkStatusType
	var mark models.Mark
	err := u.trManager.Do(ctx, func(ctx context.Context) error {
		// Same lock as AddCheck: a vote landing during the moderator's
		// decision waits for it instead of resolving the stage a second time.
		if err := u.repos.Marks.LockMark(ctx, markId); err != nil {
			return err
		}

		var checks []models.Check
		var err error
		mark, checks, err = u.loadStage(ctx, markId)
		if err != nil {
			return err
		}

		newStatus, err = transition(ctx, mark, checks)
		return err
	})
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	// Published after the commit: a rolled back decision must not notify.
	u.statusChanged(ctx, mark, newStatus)
	pending.Flush(ctx, u.log, u.events)

	return newStatus, nil
}

// loadStage reads the mark and the checks of its current voting stage
// (the ones attached to the latest status history item).
func (u *Updater) loadStage(ctx context.Context, markId int) (models.Mark, []models.Check, error) {
	mark, err := u.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return models.Mark{}, nil, err
	}

	historyItem, err := u.repos.Marks.GetLastMarkStatusHistoryItem(ctx, markId)
	if err != nil {
		return models.Mark{}, nil, err
	}

	checks, err := u.repos.Checks.GetChecksByMarkHistoryId(ctx, historyItem.ID)
	if err != nil {
		return models.Mark{}, nil, err
	}

	return mark, checks, nil
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

	// A confirmed mark is handed to the responsible city service.
	if newStatus == models.ConfirmedStatus && u.assigner != nil {
		if err := u.assigner.AssignConfirmed(ctx, mark); err != nil {
			return err
		}
	}

	markId := null.IntFrom(int64(mark.ID))

	// Rating rows are updated in user order so that two stages resolving
	// at once lock the users' rows in the same sequence and cannot deadlock.
	checks = slices.SortedFunc(slices.Values(checks), func(a, b models.Check) int {
		return cmp.Compare(a.UserID, b.UserID)
	})

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
