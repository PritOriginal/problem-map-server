package usecase

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/db"
)

type Tasks interface {
	GetTasks(ctx context.Context) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}

type TasksUseCase struct {
	tasksRepo db.TasksRepository
}

func NewTasks(tasksRepo db.TasksRepository) *TasksUseCase {
	return &TasksUseCase{tasksRepo: tasksRepo}
}

func (uc *TasksUseCase) GetTasks(ctx context.Context) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasks"

	tasks, err := uc.tasksRepo.GetTasks(ctx)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (uc *TasksUseCase) GetTaskById(ctx context.Context, id int) (models.Task, error) {
	const op = "usecase.Tasks.GetTaskById"

	task, err := uc.tasksRepo.GetTaskById(ctx, id)
	if err != nil {
		return task, fmt.Errorf("%s: %w", op, err)
	}

	return task, nil
}

func (uc *TasksUseCase) GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasksByUserId"

	tasks, err := uc.tasksRepo.GetTasksByUserId(ctx, userId)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (uc *TasksUseCase) AddTask(ctx context.Context, task models.Task) (int64, error) {
	const op = "usecase.Tasks.AddTask"

	id, err := uc.tasksRepo.AddTask(ctx, task)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}
