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

type Checks struct {
	log        *slog.Logger
	checksRepo ChecksRepository
	photosRepo PhotosRepository
}

func NewChecks(log *slog.Logger, checksRepo ChecksRepository, photosRepo PhotosRepository) *Checks {
	return &Checks{
		log:        log,
		checksRepo: checksRepo,
		photosRepo: photosRepo,
	}
}

func (uc *Checks) AddCheck(ctx context.Context, check models.Check, photos []io.Reader) (int64, error) {
	const op = "usecase.Tasks.AddCheck"

	id, err := uc.checksRepo.AddCheck(ctx, check)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	if err := uc.photosRepo.AddPhotos(ctx, check.MarkID, int(id), photos); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (uc *Checks) GetCheckById(ctx context.Context, id int) (models.Check, error) {
	const op = "usecase.Tasks.GetReviewById"

	review, err := uc.checksRepo.GetCheckById(ctx, id)
	if err != nil {
		return review, fmt.Errorf("%s: %w", op, err)
	}

	return review, nil
}

func (uc *Checks) GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error) {
	const op = "usecase.Tasks.GetReviewsByMarkId"

	reviews, err := uc.checksRepo.GetChecksByMarkId(ctx, markId)
	if err != nil {
		return reviews, fmt.Errorf("%s: %w", op, err)
	}

	return reviews, nil
}

func (uc *Checks) GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error) {
	const op = "usecase.Tasks.GetReviewsByUserId"

	reviews, err := uc.checksRepo.GetChecksByUserId(ctx, userId)
	if err != nil {
		return reviews, fmt.Errorf("%s: %w", op, err)
	}

	return reviews, nil
}
