package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type ChecksRepository interface {
	AddCheck(ctx context.Context, check models.Check) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error)
	GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error)
}

type MarkStatusUpdater interface {
	Update(ctx context.Context, markId int) error
}

type ChecksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Photos PhotosRepository
}

type Checks struct {
	log               *slog.Logger
	repos             ChecksRepositories
	markStatusUpdater MarkStatusUpdater
}

func NewChecks(log *slog.Logger, markStatusUpdater MarkStatusUpdater, repos ChecksRepositories) *Checks {
	return &Checks{
		log:               log,
		repos:             repos,
		markStatusUpdater: markStatusUpdater,
	}
}

func (uc *Checks) AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error) {
	const op = "usecase.Tasks.AddCheck"

	id, err := uc.checksRepo.AddCheck(ctx, check)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	mark, err := uc.marksRepo.GetMarkById(ctx, check.MarkID)
	if mark.MarkStatusID == int(models.UnconfirmedStatus) {
		checks, err := uc.checksRepo.GetChecksByMarkId(ctx, check.MarkID)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", op, err)
		}

		score := 0
		for _, check := range checks {
			if check.Result {
				score++
			} else {
				score--
			}
		}

		uc.log.Debug("score", slog.Int("val", score))

		if score >= 3 {
			if err := uc.marksRepo.UpdateMarkStatus(ctx, check.MarkID, models.ConfirmedStatus); err != nil {
				return 0, fmt.Errorf("%s: %w", op, err)
			}
			uc.log.Debug("change mark status", slog.Int("old", mark.MarkStatusID), slog.Int("new", int(models.ConfirmedStatus)))
		} else if score <= -3 {
			if err := uc.marksRepo.UpdateMarkStatus(ctx, check.MarkID, models.RefutedStatus); err != nil {
				return 0, fmt.Errorf("%s: %w", op, err)
			}
			uc.log.Debug("change mark status", slog.Int("old", mark.MarkStatusID), slog.Int("new", int(models.RefutedStatus)))
		}
	}

	if err := uc.photosRepo.AddPhotos(ctx, check.MarkID, int(id), photos); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (uc *Checks) GetCheckById(ctx context.Context, id int) (models.Check, error) {
	const op = "usecase.Tasks.GetCheckById"

	check, err := uc.checksRepo.GetCheckById(ctx, id)
	if err != nil {
		return check, fmt.Errorf("%s: %w", op, err)
	}

	check.Photos, err = uc.photosRepo.GetPhotosByCheckId(ctx, check.MarkID, check.ID)
	if err != nil {
		return check, fmt.Errorf("%s: %w", op, err)
	}

	return check, nil
}

func (uc *Checks) GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error) {
	const op = "usecase.Tasks.GetChecksByMarkId"

	checks, err := uc.checksRepo.GetChecksByMarkId(ctx, markId)
	if err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	photosMap, err := uc.photosRepo.GetPhotosByMarkId(ctx, markId)
	if err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	for i := range len(checks) {
		checks[i].Photos = photosMap[markId][checks[i].ID]
	}

	return checks, nil
}

func (uc *Checks) GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error) {
	const op = "usecase.Tasks.GetChecksByUserId"

	checks, err := uc.checksRepo.GetChecksByUserId(ctx, userId)
	if err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	for i := range len(checks) {
		checks[i].Photos, err = uc.photosRepo.GetPhotosByCheckId(ctx, checks[i].MarkID, checks[i].ID)
		if err != nil {
			return checks, fmt.Errorf("%s: %w", op, err)
		}
	}

	return checks, nil
}
