package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

type MarksRepository interface {
	GetMarks(ctx context.Context, filters models.GetMarksFilters) (models.Page[models.Mark], error)
	GetMarksNearby(ctx context.Context, filters models.GetMarksNearbyFilters) (models.Page[models.MarkWithDistance], error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Mark], error)
	AddMark(ctx context.Context, mark models.Mark) (int64, error)
	GetMarkTypes(ctx context.Context) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error)
	UpdateMarkStatus(ctx context.Context, markId int, markStatusId models.MarkStatusType) error
	GetMarkStatusHistoryByMarkId(ctx context.Context, markId int) ([]models.MarkStatusHistoryItem, error)
	GetLastMarkStatusHistoryItem(ctx context.Context, markId int) (models.MarkStatusHistoryItem, error)
	GetLastMarkStatusHistoryItemWithStatus(ctx context.Context, markId int, newMarkStatusId models.MarkStatusType) (models.MarkStatusHistoryItem, error)
	GetDistancesFromMarkToPoint(ctx context.Context, filters models.GetDistanceFromMarkToPointFilters) ([]models.DistanceFromMarkToPoint, error)
}

type PhotosRepository interface {
	AddPhotos(ctx context.Context, markId, checkId int, photos []io.Reader) error
	GetPhotos(ctx context.Context) (map[int]map[int][]string, error)
	GetPhotosByMarkId(ctx context.Context, markId int) (map[int]map[int][]string, error)
	GetPhotosByCheckId(ctx context.Context, markId, checkId int) ([]string, error)
}

type Marks struct {
	log       *slog.Logger
	trManager trm.Manager
	repos     MarksRepositories
}

type MarksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Photos PhotosRepository
}

func NewMarks(log *slog.Logger, trManager trm.Manager, repos MarksRepositories) *Marks {
	return &Marks{
		log:       log,
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
		return page, fmt.Errorf("%s: %w", op, err)
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
		return page.Items, fmt.Errorf("%s: %w", op, err)
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
		return page, fmt.Errorf("%s: %w", op, err)
	}
	return page, nil
}

func (uc *Marks) GetMarkById(ctx context.Context, id int) (models.Mark, error) {
	const op = "usecase.Marks.GetMarkById"

	mark, err := uc.repos.Marks.GetMarkById(ctx, id)
	if err != nil {
		return mark, fmt.Errorf("%s: %w", op, err)
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
		return page, fmt.Errorf("%s: %w", op, err)
	}
	return page, nil
}

// GetMarksByUserId returns all marks of the user without pagination (gRPC).
func (uc *Marks) GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error) {
	const op = "usecase.Marks.GetMarksByUserId"

	page, err := uc.repos.Marks.GetMarksByUserId(ctx, userId, models.Pagination{})
	if err != nil {
		return page.Items, fmt.Errorf("%s: %w", op, err)
	}
	return page.Items, nil
}

func (uc *Marks) AddMark(ctx context.Context, mark models.Mark, photos []io.Reader) (int64, error) {
	const op = "usecase.Marks.AddMark"

	var markId int64
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		var err error
		markId, err = uc.repos.Marks.AddMark(ctx, mark)
		if err != nil {
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
		return markId, fmt.Errorf("%s: %w", op, err)
	}

	return markId, nil
}

func (uc *Marks) GetMarkTypes(ctx context.Context) ([]models.MarkType, error) {
	const op = "usecase.Marks.GetMarkTypes"

	types, err := uc.repos.Marks.GetMarkTypes(ctx)
	if err != nil {
		return types, fmt.Errorf("%s: %w", op, err)
	}

	return types, nil
}

func (uc *Marks) GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error) {
	const op = "usecase.Marks.GetMarkTypes"

	statuses, err := uc.repos.Marks.GetMarkStatuses(ctx)
	if err != nil {
		return statuses, fmt.Errorf("%s: %w", op, err)
	}

	return statuses, nil
}

func (uc *Marks) GetMarkStatusHistoryByMarkId(ctx context.Context, markId int, withChecks bool) ([]models.MarkStatusHistoryItem, error) {
	const op = "usecase.Marks.GetMarkStatusHistoryByMarkId"

	historyItems, err := uc.repos.Marks.GetMarkStatusHistoryByMarkId(ctx, markId)
	if err != nil {
		return historyItems, fmt.Errorf("%s: %w", op, err)
	}

	if withChecks {
		checksPage, err := uc.repos.Checks.GetChecksByMarkId(ctx, markId, models.Pagination{})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		checks := checksPage.Items

		photosMap, err := uc.repos.Photos.GetPhotosByMarkId(ctx, markId)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
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
// 		return 0, fmt.Errorf("%s: %w", op, err)
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
// 		return 0, fmt.Errorf("%s: %w", op, err)
// 	}

// 	return newStatus, nil
// }

// func (uc *Marks) Reject(ctx context.Context, markId int) (models.MarkStatusType, error) {
// 	const op = "usecase.Map.Confirm"

// 	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
// 	if err != nil {
// 		return 0, fmt.Errorf("%s: %w", op, err)
// 	}

// 	var newStatus models.MarkStatusType

// 	switch mark.MarkStatusID {
// 	case models.UnconfirmedStatus, models.ConfirmedStatus, models.RediscoveredStatus:
// 		newStatus = models.RefutedStatus
// 	case models.UnderReviewStatus:
// 		newStatus = models.RediscoveredStatus
// 	}

// 	if err := uc.repos.Marks.UpdateMarkStatus(ctx, markId, newStatus); err != nil {
// 		return 0, fmt.Errorf("%s: %w", op, err)
// 	}

// 	return newStatus, nil
// }
