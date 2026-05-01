package postgres

import (
	"context"
	"database/sql"
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

func (r *TasksRepository) GetTasks(ctx context.Context) ([]models.Task, error) {
	const op = "storage.postgres.GetTasks"

	tasks := make([]models.Task, 0)

	query := "SELECT * FROM tasks"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &tasks, query); err != nil {
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

func (r *TasksRepository) GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error) {
	const op = "storage.postgres.GetTasksByUserId"

	tasks := []models.Task{}

	query := "SELECT * FROM tasks WHERE user_id = $1"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	err := tr.SelectContext(ctx, &tasks, query, userId)
	if err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
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
