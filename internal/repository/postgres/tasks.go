package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
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

const taskColumns = "task_id, name, user_id, mark_id, status_id, created_at, updated_at"

func (r *TasksRepository) GetTasks(ctx context.Context, filters models.GetTasksFilters) (models.Page[models.Task], error) {
	const op = "storage.postgres.GetTasks"

	q := newListQuery(taskColumns, "tasks").
		OrderBy("created_at DESC, task_id DESC").
		Paginate(filters.Pagination)

	if len(filters.Statuses) > 0 {
		q.Where("status_id IN (?)", filters.Statuses)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Task](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
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

func (r *TasksRepository) GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) (models.Page[models.Task], error) {
	const op = "storage.postgres.GetTasksByUserId"

	q := newListQuery(taskColumns, "tasks").
		Where("user_id = ?", userId).
		OrderBy("created_at DESC, task_id DESC").
		Paginate(filters.Pagination)

	if len(filters.Statuses) > 0 {
		q.Where("status_id IN (?)", filters.Statuses)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Task](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *TasksRepository) GetTaskByUserIdAndMarkId(ctx context.Context, userId int, markId int) (models.Task, error) {
	const op = "storage.postgres.GetTaskByUserIdAndMarkId"

	var task models.Task

	query := "SELECT * FROM tasks WHERE user_id = $1 AND mark_id = $2"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &task, query, userId, markId); err != nil {
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
				tasks (name, user_id, mark_id) 
			VALUES 
				($1, $2, $3)
			RETURNING task_id
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, task.Name, task.UserID, task.MarkID); err != nil {
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
