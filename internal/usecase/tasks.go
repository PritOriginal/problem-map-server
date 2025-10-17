package usecase

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/postgres"
)

type Tasks struct {
	tasksRepo postgres.TasksRepository
}

func NewTasks(tasksRepo postgres.TasksRepository) *Tasks {
	return &Tasks{tasksRepo: tasksRepo}
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
