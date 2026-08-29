package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type TasksRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewTasks(conn *sqlx.DB, c *trmsqlx.CtxGetter) *TasksRepository {
	return &TasksRepository{
		db:     conn,
		getter: c,
	}
}

func (r *TasksRepository) GetTasks(ctx context.Context, filters models.GetTasksFilters) ([]models.Task, error) {
	const op = "storage.postgres.GetTasks"

	tasks := []models.Task{}

	var conditions []string
	var args []any
	query := "SELECT * FROM tasks WHERE 1=1"

	if len(filters.Statuses) > 0 {
		conditions = append(conditions, "status_id = ANY($?)")
		args = append(args, pq.Array(filters.Statuses))
	}
	for i, condition := range conditions {
		query += " AND " + condition
		query = strings.Replace(query, "$?", fmt.Sprintf("$%d", len(args)-len(conditions)+i+1), 1)
	}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &tasks, query, args...); err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (r *TasksRepository) GetTaskById(ctx context.Context, id int) (models.Task, error) {
	const op = "storage.postgres.GetTaskById"

	var task models.Task

	query := "SELECT * FROM tasks WHERE task_id = $1"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &task, query, id); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return task, repository.ErrNotFound
		default:
			return task, fmt.Errorf("%s: %w", op, err)
		}
	}

	return task, nil
}

func (r *TasksRepository) GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) ([]models.Task, error) {
	const op = "storage.postgres.GetTasksByUserId"

	tasks := []models.Task{}

	var conditions []string
	var args []any
	query := "SELECT * FROM tasks WHERE 1=1"

	conditions = append(conditions, "user_id = $?")
	args = append(args, userId)

	if len(filters.Statuses) > 0 {
		conditions = append(conditions, "status_id = ANY($?)")
		args = append(args, pq.Array(filters.Statuses))
	}
	for i, condition := range conditions {
		query += " AND " + condition
		query = strings.Replace(query, "$?", fmt.Sprintf("$%d", len(args)-len(conditions)+i+1), 1)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	err := tr.SelectContext(ctx, &tasks, query, args...)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

// GetTaskByUserIdAndMarkId returns the user's task for the mark in the given
// status. With UnfulfilledStatus that is the single issued task
// (uq_tasks_issued_user_mark); for other statuses the latest one is returned.
func (r *TasksRepository) GetTaskByUserIdAndMarkId(ctx context.Context, userId int, markId int, statusId models.TaskStatusType) (models.Task, error) {
	const op = "storage.postgres.GetTaskByUserIdAndMarkId"

	var task models.Task

	query := `
			SELECT * FROM tasks
			WHERE user_id = $1 AND mark_id = $2 AND status_id = $3
			ORDER BY created_at DESC
			LIMIT 1
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &task, query, userId, markId, statusId); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return task, repository.ErrNotFound
		default:
			return task, fmt.Errorf("%s: %w", op, err)
		}
	}

	return task, nil
}

func (r *TasksRepository) AddTask(ctx context.Context, task models.Task) (int64, error) {
	const op = "storage.postgres.AddTask"

	var id int64

	query := `
			INSERT INTO
				tasks (name, user_id, mark_id, due_at)
			VALUES
				($1, $2, $3, $4)
			RETURNING task_id
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, task.Name, task.UserID, task.MarkID, task.DueAt); err != nil {
		return 0, fmt.Errorf("%s: %w", op, wrapUniqueViolation(err))
	}

	return id, nil
}

func (r *TasksRepository) UpdateTaskStatus(ctx context.Context, taskId int, taskStatusId models.TaskStatusType) error {
	const op = "storage.postgres.UpdateTaskStatus"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "UPDATE tasks SET status_id = $1, updated_at = NOW() WHERE task_id = $2", taskStatusId, taskId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// ExpireOverdueTasks moves every issued task whose due_at is before now to
// OverdueStatus in a single UPDATE and returns the number of affected rows.
func (r *TasksRepository) ExpireOverdueTasks(ctx context.Context, now time.Time) (int64, error) {
	const op = "storage.postgres.ExpireOverdueTasks"

	query := `
			UPDATE tasks
			SET status_id = $1, updated_at = NOW()
			WHERE status_id = $2 AND due_at IS NOT NULL AND due_at < $3
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, models.OverdueStatus, models.UnfulfilledStatus, now)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return n, nil
}
