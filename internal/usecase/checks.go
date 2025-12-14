package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type ChecksRepository interface {
	AddCheck(ctx context.Context, review models.Check) (int64, error)
	GetCheckById(ctx context.Context, id int) (models.Check, error)
	GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error)
	GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error)
}

type Checks struct {
	log        *slog.Logger
	checksRepo ChecksRepository
}

func NewChecks(log *slog.Logger, checksRepo ChecksRepository) *Checks {
	return &Checks{
		log:        log,
		checksRepo: checksRepo,
	}
}

func (uc *Checks) AddCheck(ctx context.Context, review models.Check) (int64, error) {
	const op = "usecase.Tasks.AddReview"

	id, err := uc.checksRepo.AddCheck(ctx, review)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
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
