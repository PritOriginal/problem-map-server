package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/jmoiron/sqlx"
)

type TasksRepository struct {
	Conn *sqlx.DB
}

func NewTasks(conn *sqlx.DB) *TasksRepository {
	return &TasksRepository{Conn: conn}
}

func (r *TasksRepository) GetTasks(ctx context.Context) ([]models.Task, error) {
	const op = "storage.postgres.GetTasks"

	tasks := make([]models.Task, 0)

	query := "SELECT * FROM tasks"
	if err := r.Conn.SelectContext(ctx, &tasks, query); err != nil {
		return tasks, fmt.Errorf("%s: %w", op, err)
	}

	return tasks, nil
}

func (r *TasksRepository) GetTaskById(ctx context.Context, id int) (models.Task, error) {
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

func (r *TasksRepository) GetTasksByUserId(ctx context.Context, userId int) ([]models.Task, error) {
	const op = "storage.postgres.GetTasksByUserId"

	tasks := []models.Task{}

	query := "SELECT * FROM tasks WHERE user_id = $1"
	err := r.Conn.SelectContext(ctx, &tasks, query, userId)
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

	stmt, err := r.Conn.PrepareNamedContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := stmt.GetContext(ctx, &id, task); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (r *TasksRepository) GetReviewById(ctx context.Context, id int) (models.Review, error) {
	const op = "storage.postgres.GetReviewById"

	var review models.Review

	query := "SELECT * FROM reviews WHERE review_id = $1"
	if err := r.Conn.GetContext(ctx, &review, query, id); err != nil {
		return review, fmt.Errorf("%s: %w", op, err)
	}

	return review, nil
}

func (r *TasksRepository) GetReviewsByMarkId(ctx context.Context, markId int) ([]models.Review, error) {
	const op = "storage.postgres.GetReviewsByMarkId"

	var reviews []models.Review

	query := "SELECT * FROM reviews WHERE mark_id = $1"
	if err := r.Conn.SelectContext(ctx, &reviews, query, markId); err != nil {
		return reviews, fmt.Errorf("%s: %w", op, err)
	}

	return reviews, nil
}

func (r *TasksRepository) GetReviewsByUserId(ctx context.Context, userId int) ([]models.Review, error) {
	const op = "storage.postgres.GetReviewsByUserId"

	var reviews []models.Review

	query := "SELECT * FROM reviews WHERE user_id = $1"
	if err := r.Conn.SelectContext(ctx, &reviews, query, userId); err != nil {
		return reviews, fmt.Errorf("%s: %w", op, err)
	}

	return reviews, nil
}
