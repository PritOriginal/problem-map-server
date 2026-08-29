package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
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

// tasksColumns qualifies taskColumns with the tasks alias for joined queries.
const tasksColumns = "t.task_id, t.name, t.user_id, t.mark_id, t.status_id, t.created_at, t.updated_at"

func (r *TasksRepository) GetTasks(ctx context.Context, filters models.GetTasksFilters) (models.Page[models.Task], error) {
	const op = "storage.postgres.GetTasks"

	q := newListQuery(tasksColumns, "tasks t").
		OrderBy("t.created_at DESC, t.task_id DESC").
		Paginate(filters.Pagination)

	if len(filters.Statuses) > 0 {
		q.Where("t.status_id IN (?)", filters.Statuses)
	}
	if len(filters.MarkStatusIds) > 0 {
		q.from = "tasks t JOIN marks m ON m.mark_id = t.mark_id"
		q.Where("m.mark_status_id IN (?)", filters.MarkStatusIds)
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
		return task, wrapPgError(op, err)
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
		return task, wrapPgError(op, err)
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
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

// GetTaskStatuses lists the task statuses with names in lang (falling back
// to the default language, then to the raw name).
func (r *TasksRepository) GetTaskStatuses(ctx context.Context, lang models.Lang) ([]models.TaskStatus, error) {
	const op = "storage.postgres.GetTaskStatuses"

	statuses := []models.TaskStatus{}

	query := `SELECT s.status_id, s.code, ` + translatedName("s.name") + `
		FROM task_statuses s
		` + translationJoins("task_status", "s.status_id") + `
		ORDER BY s.status_id`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &statuses, query, lang, models.DefaultLang); err != nil {
		return statuses, fmt.Errorf("%s: %w", op, err)
	}

	return statuses, nil
}

func (r *TasksRepository) UpdateTaskStatus(ctx context.Context, taskId int, taskStatusId models.TaskStatusType) error {
	const op = "storage.postgres.UpdateTaskStatus"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "UPDATE tasks SET status_id = $1, updated_at = NOW() WHERE task_id = $2", taskStatusId, taskId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// MoveOpenTasks moves the issued tasks of the mark to target (a merge of
// duplicates). A task whose user already holds an issued task on the
// target, or who is the author of the target, is deleted instead. Call it
// inside a transaction.
func (r *TasksRepository) MoveOpenTasks(ctx context.Context, markId, targetId int) error {
	const op = "storage.postgres.MoveOpenTasks"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, `
		DELETE FROM tasks t
		WHERE t.mark_id = $1 AND t.status_id = $3
			AND (
				EXISTS (SELECT 1 FROM tasks t2 WHERE t2.mark_id = $2 AND t2.user_id = t.user_id AND t2.status_id = $3)
				OR EXISTS (SELECT 1 FROM marks m WHERE m.mark_id = $2 AND m.user_id = t.user_id)
			)`, markId, targetId, models.UnfulfilledStatus); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if _, err := tr.ExecContext(ctx, `
		UPDATE tasks SET mark_id = $2, updated_at = NOW()
		WHERE mark_id = $1 AND status_id = $3`, markId, targetId, models.UnfulfilledStatus); err != nil {
		return wrapPgError(op, err)
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
