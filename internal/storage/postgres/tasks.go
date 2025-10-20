package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/jmoiron/sqlx"
)

type TasksRepository interface {
	GetTasks(ctx context.Context) ([]models.Task, error)
	GetTaskById(ctx context.Context, id int) (models.Task, error)
	GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error)
	AddTask(ctx context.Context, task models.Task) (int64, error)
}

type TasksRepo struct {
	Conn *sqlx.DB
}

func NewTasks(conn *sqlx.DB) *TasksRepo {
	return &TasksRepo{Conn: conn}
}

func (r *TasksRepo) GetTasks(ctx context.Context) ([]models.Task, error) {
	const op = "storage.postgres.GetTasks"

	tasks := make([]models.Task, 0)

	query := "SELECT * FROM tasks"
	if err := r.Conn.SelectContext(ctx, &tasks, query); err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (r *TasksRepo) GetTaskById(ctx context.Context, id int) (models.Task, error) {
	const op = "storage.postgres.GetTaskById"

	var task models.Task

	query := "SELECT * FROM tasks WHERE task_id = $1"
	if err := r.Conn.GetContext(ctx, &task, query, id); err != nil {
		switch err {
		case sql.ErrNoRows:
			return task, storage.ErrNotFound
		default:
			return task, fmt.Errorf("%s: %w", op, err)
		}
	}

	return task, nil
}

func (r *TasksRepo) GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error) {
	const op = "storage.postgres.GetTasksByUserId"

	var tasks []models.Task

	query := "SELECT * FROM tasks WHERE user_id = $1"
	err := r.Conn.SelectContext(ctx, &tasks, query, userId)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}
func (r *TasksRepo) AddTask(ctx context.Context, task models.Task) (int64, error) {
	const op = "storage.postgres.AddTask"

	result, err := r.Conn.NamedExecContext(ctx, "INSERT INTO tasks (name, user_id) VALUES (:name, :user_id)", task)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return id, nil
}
