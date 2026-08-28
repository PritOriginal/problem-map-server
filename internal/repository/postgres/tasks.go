package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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
		switch err {
		case sql.ErrNoRows:
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

func (r *TasksRepository) GetTaskByUserIdAndMarkId(ctx context.Context, userId int, markId int) (models.Task, error) {
	const op = "storage.postgres.GetTaskByUserIdAndMarkId"

	var task models.Task

	query := "SELECT * FROM tasks WHERE user_id = $1 AND mark_id = $2"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &task, query, userId, markId); err != nil {
		switch err {
		case sql.ErrNoRows:
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
				tasks (name, user_id, mark_id) 
			VALUES 
				(:name, :user_id, :mark_id)
			RETURNING task_id
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	stmt, err := tr.PrepareNamedContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := stmt.GetContext(ctx, &id, task); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (r *TasksRepository) UpdateTaskStatus(ctx context.Context, taskId int, taskStatusId models.TaskStatusType) error {
	const op = "storage.postgres.UpdateTaskStatus"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "UPDATE tasks SET status_id = $1 WHERE task_id = $2", taskStatusId, taskId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
