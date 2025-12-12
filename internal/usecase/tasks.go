package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type TasksRepository interface {
	GetTasks(ctx context.Context) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
	AddReview(ctx context.Context, review models.Review) (int64, error)
	GetReviewById(ctx context.Context, id int) (models.Review, error)
	GetReviewsByMarkId(ctx context.Context, markId int) ([]models.Review, error)
	GetReviewsByUserId(ctx context.Context, userId int) ([]models.Review, error)
}

type Tasks struct {
	log       *slog.Logger
	tasksRepo TasksRepository
}

func NewTasks(log *slog.Logger, tasksRepo TasksRepository) *Tasks {
	return &Tasks{log: log, tasksRepo: tasksRepo}
}

func (uc *Tasks) GetTasks(ctx context.Context) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasks"

	tasks, err := uc.tasksRepo.GetTasks(ctx)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (uc *Tasks) GetTaskById(ctx context.Context, id int) (models.Task, error) {
	const op = "usecase.Tasks.GetTaskById"

	task, err := uc.tasksRepo.GetTaskById(ctx, id)
	if err != nil {
		return task, fmt.Errorf("%s: %w", op, err)
	}

	return task, nil
}

func (uc *Tasks) GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasksByUserId"

	tasks, err := uc.tasksRepo.GetTasksByUserId(ctx, userId)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (uc *Tasks) AddTask(ctx context.Context, task models.Task) (int64, error) {
	const op = "usecase.Tasks.AddTask"

	id, err := uc.tasksRepo.AddTask(ctx, task)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (uc *Tasks) AddReview(ctx context.Context, review models.Review) (int64, error) {
	const op = "usecase.Tasks.AddReview"

	id, err := uc.tasksRepo.AddReview(ctx, review)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (uc *Tasks) GetReviewById(ctx context.Context, id int) (models.Review, error) {
	const op = "usecase.Tasks.GetReviewById"

	review, err := uc.tasksRepo.GetReviewById(ctx, id)
	if err != nil {
		return review, fmt.Errorf("%s: %w", op, err)
	}

	return review, nil
}

func (uc *Tasks) GetReviewsByMarkId(ctx context.Context, markId int) ([]models.Review, error) {
	const op = "usecase.Tasks.GetReviewsByMarkId"

	reviews, err := uc.tasksRepo.GetReviewsByMarkId(ctx, markId)
	if err != nil {
		return reviews, fmt.Errorf("%s: %w", op, err)
	}

	return reviews, nil
}

func (uc *Tasks) GetReviewsByUserId(ctx context.Context, userId int) ([]models.Review, error) {
	const op = "usecase.Tasks.GetReviewsByUserId"

	reviews, err := uc.tasksRepo.GetReviewsByUserId(ctx, userId)
	if err != nil {
		return reviews, fmt.Errorf("%s: %w", op, err)
	}

	return reviews, nil
}
