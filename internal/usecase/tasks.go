package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type TasksRepository interface {
	GetTasks(ctx context.Context, filters models.GetTasksFilters) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) ([]models.Task, error)
	GetTaskByUserIdAndMarkId(ctx context.Context, userId int, markId int) (models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
	UpdateTaskStatus(ctx context.Context, taskId int, taskStatusId models.TaskStatusType) error
}

type Tasks struct {
	log   *slog.Logger
	repos TasksRepositories
}

type TasksRepositories struct {
	Tasks TasksRepository
}

func NewTasks(log *slog.Logger, repos TasksRepositories) *Tasks {
	return &Tasks{log: log, repos: repos}
}

func (uc *Tasks) GetTasks(ctx context.Context, filters models.GetTasksFilters) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasks"

	tasks, err := uc.repos.Tasks.GetTasks(ctx, filters)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (uc *Tasks) GetTaskById(ctx context.Context, id int) (models.Task, error) {
	const op = "usecase.Tasks.GetTaskById"

	task, err := uc.repos.Tasks.GetTaskById(ctx, id)
	if err != nil {
		return task, fmt.Errorf("%s: %w", op, err)
	}

	return task, nil
}

func (uc *Tasks) GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) ([]models.Task, error) {
	const op = "usecase.Tasks.GetTasksByUserId"

	tasks, err := uc.repos.Tasks.GetTasksByUserId(ctx, userId, filters)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (uc *Tasks) AddTask(ctx context.Context, task models.Task) (int64, error) {
	const op = "usecase.Tasks.AddTask"

	id, err := uc.repos.Tasks.AddTask(ctx, task)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}
