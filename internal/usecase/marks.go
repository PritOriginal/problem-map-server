package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type MarksRepository interface {
	GetMarks(ctx context.Context) ([]models.Mark, error)
	GetMarkById(ctx context.Context, id int) (models.Mark, error)
	GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark) (int64, error)
	GetMarkTypes(ctx context.Context) ([]models.MarkType, error)
	GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error)
	UpdateMarkStatus(ctx context.Context, markId int, markStatusId models.MarkStatusType) error
}

type PhotosRepository interface {
	AddPhotos(ctx context.Context, markId, checkId int, photos []io.Reader) error
	GetPhotos(ctx context.Context) (map[int]map[int][]string, error)
	GetPhotosByMarkId(ctx context.Context, markId int) (map[int]map[int][]string, error)
	GetPhotosByCheckId(ctx context.Context, markId, checkId int) ([]string, error)
}

type Marks struct {
	log   *slog.Logger
	repos MarksRepositories
}

type MarksRepositories struct {
	Marks  MarksRepository
	Checks ChecksRepository
	Photos PhotosRepository
}

func NewMarks(log *slog.Logger, repos MarksRepositories) *Marks {
	return &Marks{
		log:   log,
		repos: repos,
	}
}

func (uc *Marks) GetMarks(ctx context.Context) ([]models.Mark, error) {
	const op = "usecase.Map.GetMarks"

	marks, err := uc.repos.Marks.GetMarks(ctx)
	if err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}
	return marks, nil
}

func (uc *Marks) GetMarkById(ctx context.Context, id int) (models.Mark, error) {
	const op = "usecase.Map.GetMarkById"

	mark, err := uc.repos.Marks.GetMarkById(ctx, id)
	if err != nil {
		return mark, fmt.Errorf("%s: %w", op, err)
	}
	return mark, nil
}

func (uc *Marks) GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error) {
	const op = "usecase.Map.GetMarksByUserId"

	marks, err := uc.repos.Marks.GetMarksByUserId(ctx, userId)
	if err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}
	return marks, nil
}

func (uc *Marks) AddMark(ctx context.Context, mark models.Mark, photos []io.Reader) (int64, error) {
	const op = "usecase.Map.AddMark"

	markId, err := uc.repos.Marks.AddMark(ctx, mark)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	check := models.Check{
		UserID:  mark.UserID,
		MarkID:  int(markId),
		Result:  true,
		Comment: mark.Description,
	}

	checkId, err := uc.repos.Checks.AddCheck(ctx, check)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := uc.repos.Photos.AddPhotos(ctx, int(markId), int(checkId), photos); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return markId, nil
}

func (uc *Marks) GetMarkTypes(ctx context.Context) ([]models.MarkType, error) {
	const op = "usecase.Map.GetMarkTypes"

	types, err := uc.repos.Marks.GetMarkTypes(ctx)
	if err != nil {
		return types, fmt.Errorf("%s: %w", op, err)
	}

	return types, nil
}

func (uc *Marks) GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error) {
	const op = "usecase.Map.GetMarkTypes"

	statuses, err := uc.repos.Marks.GetMarkStatuses(ctx)
	if err != nil {
		return statuses, fmt.Errorf("%s: %w", op, err)
	}

	return statuses, nil
}
