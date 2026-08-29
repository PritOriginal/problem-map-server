package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

type MarksRepository interface {
	GetMarks(ctx context.Context, filters models.GetMarksFilters) (models.Page[models.Mark], error)
	GetMarksNearby(ctx context.Context, filters models.GetMarksNearbyFilters) (models.Page[models.MarkWithDistance], error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error)
	AddMark(ctx context.Context, mark models.Mark) (int64, error)
	GetMarkTypes(ctx context.Context, lang models.Lang) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context, lang models.Lang) ([]models.MarkStatus, error)
	// LockMark locks the mark row for the rest of the transaction
	// (repository.ErrNotFound when the mark does not exist).
	LockMark(ctx context.Context, markId int) error
	UpdateMarkStatus(ctx context.Context, markId int, markStatusId models.MarkStatusType) error
	GetMarkStatusHistoryByMarkId(ctx context.Context, markId int) ([]models.MarkStatusHistoryItem, error)
	GetLastMarkStatusHistoryItem(ctx context.Context, markId int) (models.MarkStatusHistoryItem, error)
	GetLastMarkStatusHistoryItemWithStatus(ctx context.Context, markId int, newMarkStatusId models.MarkStatusType) (models.MarkStatusHistoryItem, error)
	GetDistancesFromMarkToPoint(ctx context.Context, filters models.GetDistanceFromMarkToPointFilters) ([]models.DistanceFromMarkToPoint, error)
	GetSimilarMarks(ctx context.Context, filters models.GetSimilarMarksFilters) ([]models.MarkWithDistance, error)
	UpdateMark(ctx context.Context, markId int, upd models.MarkUpdate) error
	DeleteMark(ctx context.Context, markId int) error
	// GetDeletedMarkIDs pages the tombstones written by DeleteMark after since.
	GetDeletedMarkIDs(ctx context.Context, since time.Time, p models.Pagination) (models.Page[int], error)
	FollowMark(ctx context.Context, userId, markId int) error
	UnfollowMark(ctx context.Context, userId, markId int) error
	GetFollowedMarks(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error)
	GetFollowerIDs(ctx context.Context, markId int) ([]int, error)
}

type PhotosRepository interface {
	AddPhotos(ctx context.Context, markId, checkId int, photos []io.Reader) error
	DeletePhotos(ctx context.Context, markId int) error
	GetPhotos(ctx context.Context) (map[int]map[int][]string, error)
	GetPhotosByMarkId(ctx context.Context, markId int) (map[int]map[int][]string, error)
	GetPhotosByCheckId(ctx context.Context, markId, checkId int) ([]string, error)
}

type Marks struct {
	log       *slog.Logger
	cfg       config.MarksConfig
	trManager trm.Manager
	repos     MarksRepositories
}

// SimilarMarksError is returned by AddMark when active marks of the same
// type already exist nearby and the caller did not force creation. It
// matches ErrConflict via errors.Is; handlers may errors.As it to expose the
// candidates.
type SimilarMarksError struct {
	Marks []models.MarkWithDistance
}

func (e *SimilarMarksError) Error() string {
	return fmt.Sprintf("%d similar marks nearby", len(e.Marks))
}

// Is makes the error match ErrConflict.
func (e *SimilarMarksError) Is(target error) bool {
	return target == ErrConflict
}

type MarksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Photos PhotosRepository
}

func NewMarks(log *slog.Logger, cfg config.MarksConfig, trManager trm.Manager, repos MarksRepositories) *Marks {
	if cfg.DedupRadiusM <= 0 {
		cfg.DedupRadiusM = models.DefaultDedupRadiusM
	}
	return &Marks{
		log:       log,
		cfg:       cfg,
		trManager: trManager,
		repos:     repos,
	}
}

// ListMarks returns a page of marks matching the filters together with the
// total number of matches. Sort/order and pagination are validated here so
// that every transport gets the same rules.
func (uc *Marks) ListMarks(ctx context.Context, filters models.GetMarksFilters) (models.Page[models.Mark], error) {
	const op = "usecase.Marks.ListMarks"

	if err := filters.Validate(); err != nil {
		return models.Page[models.Mark]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Marks.GetMarks(ctx, filters)
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

// GetMarks returns every mark matching the filters without pagination.
// Kept for callers that need the full set (gRPC, tasker).
func (uc *Marks) GetMarks(ctx context.Context, filters models.GetMarksFilters) ([]models.Mark, error) {
	const op = "usecase.Marks.GetMarks"

	filters.Pagination = models.Pagination{}
	page, err := uc.repos.Marks.GetMarks(ctx, filters)
	if err != nil {
		return page.Items, mapRepoErr(op, err)
	}
	return page.Items, nil
}

// MaxNearbyRadiusM caps the radius accepted by GetMarksNearby (50 km).
const MaxNearbyRadiusM = models.MaxNearbyRadiusM

// GetMarksNearby returns marks within filters.RadiusM meters of the point,
// nearest first, with the distance to each mark.
func (uc *Marks) GetMarksNearby(ctx context.Context, filters models.GetMarksNearbyFilters) (models.Page[models.MarkWithDistance], error) {
	const op = "usecase.Marks.GetMarksNearby"

	if err := filters.Validate(); err != nil {
		return models.Page[models.MarkWithDistance]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Marks.GetMarksNearby(ctx, filters)
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

func (uc *Marks) GetMarkById(ctx context.Context, id int) (models.Mark, error) {
	const op = "usecase.Marks.GetMarkById"

	mark, err := uc.repos.Marks.GetMarkById(ctx, id)
	if err != nil {
		return mark, mapRepoErr(op, err)
	}
	return mark, nil
}

// ListMarksByUserId returns a page of the user's marks with the total count.
func (uc *Marks) ListMarksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error) {
	const op = "usecase.Marks.ListMarksByUserId"

	if err := p.Validate(); err != nil {
		return models.Page[models.Mark]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Marks.GetMarksByUserId(ctx, userId, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

// GetMarksByUserId returns all marks of the user without pagination (gRPC).
func (uc *Marks) GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error) {
	const op = "usecase.Marks.GetMarksByUserId"

	page, err := uc.repos.Marks.GetMarksByUserId(ctx, userId, models.Pagination{})
	if err != nil {
		return page.Items, mapRepoErr(op, err)
	}
	return page.Items, nil
}

// FindSimilarMarks returns active marks of the same type near the point.
// A zero radius means the configured dedup radius.
func (uc *Marks) FindSimilarMarks(ctx context.Context, filters models.GetSimilarMarksFilters) ([]models.MarkWithDistance, error) {
	const op = "usecase.Marks.FindSimilarMarks"

	if filters.RadiusM == 0 {
		filters.RadiusM = uc.cfg.DedupRadiusM
	}
	if err := filters.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	marks, err := uc.repos.Marks.GetSimilarMarks(ctx, filters)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return marks, nil
}

// AddMark creates the mark with its first check and photos and subscribes
// the author to it. Unless force is set, active marks of the same type
// within the dedup radius block creation with a *SimilarMarksError.
func (uc *Marks) AddMark(ctx context.Context, mark models.Mark, photos []io.Reader, force bool) (int64, error) {
	const op = "usecase.Marks.AddMark"

	if !force {
		if mark.Geom == nil {
			return 0, fmt.Errorf("%s: %w: geometry is required", op, ErrInvalidArgument)
		}
		similar, err := uc.repos.Marks.GetSimilarMarks(ctx, models.GetSimilarMarksFilters{
			Lon:        mark.Geom.Ewkb.X(),
			Lat:        mark.Geom.Ewkb.Y(),
			MarkTypeID: mark.MarkTypeID,
			RadiusM:    uc.cfg.DedupRadiusM,
		})
		if err != nil {
			return 0, mapRepoErr(op, err)
		}
		if len(similar) > 0 {
			return 0, fmt.Errorf("%s: %w", op, &SimilarMarksError{Marks: similar})
		}
	}

	var markId int64
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		var err error
		markId, err = uc.repos.Marks.AddMark(ctx, mark)
		if err != nil {
			return err
		}

		if err := uc.repos.Marks.FollowMark(ctx, mark.UserID, int(markId)); err != nil {
			return err
		}

		historyItem, err := uc.repos.Marks.GetLastMarkStatusHistoryItem(ctx, int(markId))
		if err != nil {
			return err
		}

		check := models.Check{
			UserID:                  mark.UserID,
			MarkID:                  int(markId),
			MarkStatusId:            models.UnconfirmedStatus,
			MarkStatusHistoryItemId: historyItem.ID,
			Result:                  true,
			Comment:                 mark.Description,
		}

		checkId, err := uc.repos.Checks.AddCheck(ctx, check)
		if err != nil {
			return err
		}

		if err := uc.repos.Photos.AddPhotos(ctx, int(markId), int(checkId), photos); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return markId, mapRepoErr(op, err)
	}

	return markId, nil
}

// UpdateMark changes description/type. The owner may edit only an
// unconfirmed mark; moderators may edit any mark.
func (uc *Marks) UpdateMark(ctx context.Context, actor models.Actor, markId int, upd models.MarkUpdate) (models.Mark, error) {
	const op = "usecase.Marks.UpdateMark"

	if upd.IsEmpty() {
		return models.Mark{}, fmt.Errorf("%s: %w: nothing to update", op, ErrInvalidArgument)
	}

	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}
	if err := uc.authorizeOwnerAction(actor, mark); err != nil {
		return models.Mark{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := uc.repos.Marks.UpdateMark(ctx, markId, upd); err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}

	mark, err = uc.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}
	return mark, nil
}

// DeleteMark removes the mark with its checks, tasks, history, followers
// and photos. The owner may delete only an unconfirmed mark nobody else has
// checked; moderators may delete any mark.
func (uc *Marks) DeleteMark(ctx context.Context, actor models.Actor, markId int) error {
	const op = "usecase.Marks.DeleteMark"

	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if err := uc.authorizeOwnerAction(actor, mark); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if !actor.IsModerator() {
		checks, err := uc.repos.Checks.GetChecksByMarkId(ctx, markId, models.Pagination{})
		if err != nil {
			return mapRepoErr(op, err)
		}
		for _, check := range checks.Items {
			if check.UserID != mark.UserID {
				return fmt.Errorf("%s: %w: mark has checks by other users", op, ErrConflict)
			}
		}
	}

	err = uc.trManager.Do(ctx, func(ctx context.Context) error {
		return uc.repos.Marks.DeleteMark(ctx, markId)
	})
	if err != nil {
		return mapRepoErr(op, err)
	}
	uc.log.Info("mark deleted", slog.Int("mark_id", markId), slog.Int("user_id", actor.UserID))

	// Photos live outside the database, so they are removed only after the
	// rows are committed: a rollback must not leave a mark without photos.
	// The mark is already gone, so a failure here only leaves orphaned
	// objects behind and is logged instead of failing the request.
	if err := uc.repos.Photos.DeletePhotos(ctx, markId); err != nil {
		uc.log.Warn("failed to delete mark photos", slog.Int("mark_id", markId), logger.Err(err))
	}
	return nil
}

// authorizeOwnerAction allows moderators always and the owner only while the
// mark is unconfirmed: ErrForbidden for strangers, ErrConflict for a wrong
// status.
func (uc *Marks) authorizeOwnerAction(actor models.Actor, mark models.Mark) error {
	if actor.IsModerator() {
		return nil
	}
	if mark.UserID != actor.UserID {
		return ErrForbidden
	}
	if mark.MarkStatusID != models.UnconfirmedStatus {
		return fmt.Errorf("%w: mark is not unconfirmed", ErrConflict)
	}
	return nil
}

// FollowMark subscribes the user to the mark's updates.
func (uc *Marks) FollowMark(ctx context.Context, userId, markId int) error {
	const op = "usecase.Marks.FollowMark"

	if err := uc.repos.Marks.FollowMark(ctx, userId, markId); err != nil {
		return mapRepoErr(op, err)
	}
	return nil
}

// UnfollowMark removes the subscription; the mark must exist.
func (uc *Marks) UnfollowMark(ctx context.Context, userId, markId int) error {
	const op = "usecase.Marks.UnfollowMark"

	if _, err := uc.repos.Marks.GetMarkById(ctx, markId); err != nil {
		return mapRepoErr(op, err)
	}
	if err := uc.repos.Marks.UnfollowMark(ctx, userId, markId); err != nil {
		return mapRepoErr(op, err)
	}
	return nil
}

// ListFollowedMarks returns a page of marks the user follows.
func (uc *Marks) ListFollowedMarks(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error) {
	const op = "usecase.Marks.ListFollowedMarks"

	if err := p.Validate(); err != nil {
		return models.Page[models.Mark]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Marks.GetFollowedMarks(ctx, userId, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

// GetMarkChanges returns what an offline client missed since
// filters.Since: marks updated after it (oldest change first) and ids of
// marks deleted after it, each paginated independently. ServerTime is
// taken before the queries, so using it as the next Since cannot skip a
// concurrent change on this instance; see the handler docs about clock
// skew between instances.
func (uc *Marks) GetMarkChanges(ctx context.Context, filters models.MarkChangesFilters) (models.MarkChanges, error) {
	const op = "usecase.Marks.GetMarkChanges"

	if err := filters.Validate(); err != nil {
		return models.MarkChanges{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	changes := models.MarkChanges{ServerTime: time.Now().UTC()}

	page, err := uc.repos.Marks.GetMarks(ctx, models.GetMarksFilters{
		UpdatedSince: filters.Since,
		Sort:         models.MarksSortUpdatedAt,
		Order:        models.SortAsc,
		Pagination:   filters.Pagination,
	})
	if err != nil {
		return models.MarkChanges{}, mapRepoErr(op, err)
	}
	changes.Marks, changes.Total = page.Items, page.Total

	deleted, err := uc.repos.Marks.GetDeletedMarkIDs(ctx, filters.Since, filters.Pagination)
	if err != nil {
		return models.MarkChanges{}, mapRepoErr(op, err)
	}
	changes.DeletedIDs, changes.DeletedTotal = deleted.Items, deleted.Total
	// TODO: fill HiddenIDs once marks get a hidden flag (moderation); until
	// then hidden marks are indistinguishable from visible ones.
	changes.HiddenIDs = []int{}

	return changes, nil
}

// GetMarkTypes lists the mark types with names localised to lang.
func (uc *Marks) GetMarkTypes(ctx context.Context, lang models.Lang) ([]models.MarkType, error) {
	const op = "usecase.Marks.GetMarkTypes"

	types, err := uc.repos.Marks.GetMarkTypes(ctx, lang)
	if err != nil {
		return types, mapRepoErr(op, err)
	}

	return types, nil
}

// GetMarkStatuses lists the mark statuses with names localised to lang.
func (uc *Marks) GetMarkStatuses(ctx context.Context, lang models.Lang) ([]models.MarkStatus, error) {
	const op = "usecase.Marks.GetMarkStatuses"

	statuses, err := uc.repos.Marks.GetMarkStatuses(ctx, lang)
	if err != nil {
		return statuses, mapRepoErr(op, err)
	}

	return statuses, nil
}

func (uc *Marks) GetMarkStatusHistoryByMarkId(ctx context.Context, markId int, withChecks bool) ([]models.MarkStatusHistoryItem, error) {
	const op = "usecase.Marks.GetMarkStatusHistoryByMarkId"

	historyItems, err := uc.repos.Marks.GetMarkStatusHistoryByMarkId(ctx, markId)
	if err != nil {
		return historyItems, mapRepoErr(op, err)
	}

	if withChecks {
		checksPage, err := uc.repos.Checks.GetChecksByMarkId(ctx, markId, models.Pagination{})
		if err != nil {
			return nil, mapRepoErr(op, err)
		}
		checks := checksPage.Items

		photosMap, err := uc.repos.Photos.GetPhotosByMarkId(ctx, markId)
		if err != nil {
			return nil, mapRepoErr(op, err)
		}

		for i := range len(checks) {
			if photos, ok := photosMap[markId][checks[i].ID]; ok {
				checks[i].Photos = photos
			} else {
				checks[i].Photos = []string{}
			}
		}

		historyItems = uc.addChecksToHistoryItems(historyItems, checks)
	}

	return historyItems, nil
}

func (uc *Marks) addChecksToHistoryItems(historyItems []models.MarkStatusHistoryItem, checks []models.Check) []models.MarkStatusHistoryItem {
	groupedChecksMap := make(map[int][]models.Check, len(historyItems))
	for _, check := range checks {
		historyItemId := check.MarkStatusHistoryItemId
		groupedChecksMap[historyItemId] = append(groupedChecksMap[historyItemId], check)
	}

	for i := range historyItems {
		if _, ok := groupedChecksMap[historyItems[i].ID]; ok {
			historyItems[i].Checks = groupedChecksMap[historyItems[i].ID]
		} else {
			historyItems[i].Checks = []models.Check{}
		}
	}

	return historyItems
}

// func (uc *Marks) Confirm(ctx context.Context, markId int) (models.MarkStatusType, error) {
// 	const op = "usecase.Map.Confirm"

// 	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
// 	if err != nil {
// 		return 0, mapRepoErr(op, err)
// 	}

// 	var newStatus models.MarkStatusType

// 	switch mark.MarkStatusID {
// 	case models.UnconfirmedStatus:
// 		newStatus = models.ConfirmedStatus
// 	case models.ConfirmedStatus, models.RediscoveredStatus:
// 		newStatus = models.UnderReviewStatus
// 	case models.UnderReviewStatus:
// 		newStatus = models.ClosedStatus
// 	default:
// 		return 0, ErrConflict
// 	}

// 	if err := uc.repos.Marks.UpdateMarkStatus(ctx, markId, newStatus); err != nil {
// 		return 0, mapRepoErr(op, err)
// 	}

// 	return newStatus, nil
// }

// func (uc *Marks) Reject(ctx context.Context, markId int) (models.MarkStatusType, error) {
// 	const op = "usecase.Map.Confirm"

// 	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
// 	if err != nil {
// 		return 0, mapRepoErr(op, err)
// 	}

// 	var newStatus models.MarkStatusType

// 	switch mark.MarkStatusID {
// 	case models.UnconfirmedStatus, models.ConfirmedStatus, models.RediscoveredStatus:
// 		newStatus = models.RefutedStatus
// 	case models.UnderReviewStatus:
// 		newStatus = models.RediscoveredStatus
// 	}

// 	if err := uc.repos.Marks.UpdateMarkStatus(ctx, markId, newStatus); err != nil {
// 		return 0, mapRepoErr(op, err)
// 	}

// 	return newStatus, nil
// }
