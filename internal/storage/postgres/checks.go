package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/jmoiron/sqlx"
)

type ChecksRepository struct {
	Conn *sqlx.DB
}

func NewChecks(conn *sqlx.DB) *ChecksRepository {
	return &ChecksRepository{Conn: conn}
}

func (r *ChecksRepository) AddCheck(ctx context.Context, check models.Check) (int64, error) {
	const op = "storage.postgres.AddCheck"

	var id int64

	query := `
			INSERT INTO 
				checks (user_id, mark_id, mark_status_id, mark_status_history_id, comment, result) 
			VALUES 
				(:user_id, :mark_id, :mark_status_id, :mark_status_history_id, :comment, :result)
			RETURNING check_id
			`

	stmt, err := r.Conn.PrepareNamedContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := stmt.GetContext(ctx, &id, check); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (r *ChecksRepository) GetCheckById(ctx context.Context, id int) (models.Check, error) {
	const op = "storage.postgres.GetCheckById"

	var check models.Check

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			check_id = $1`

	if err := r.Conn.GetContext(ctx, &check, query, id); err != nil {
		switch err {
		case sql.ErrNoRows:
			return check, storage.ErrNotFound
		default:
			return check, fmt.Errorf("%s: %w", op, err)
		}
	}

	return check, nil
}

func (r *ChecksRepository) GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error) {
	const op = "storage.postgres.GetChecksByMarkId"

	checks := []models.Check{}

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			mark_id = $1
		ORDER BY created_at ASC`

	if err := r.Conn.SelectContext(ctx, &checks, query, markId); err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	return checks, nil
}

func (r *ChecksRepository) GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error) {
	const op = "storage.postgres.GetChecksByUserId"

	checks := []models.Check{}

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			c.user_id = $1`

	if err := r.Conn.SelectContext(ctx, &checks, query, userId); err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	return checks, nil
}

func (r *ChecksRepository) GetUserMarkCheck(ctx context.Context, userId int, markStatusHistoryId int) (models.Check, error) {
	const op = "storage.postgres.GetUserMarkCheck"

	check := models.Check{}

	query := `
		WITH RECURSIVE r AS (
			SELECT h.id, h.prev_id, ms.parent_id, ms.name 
			FROM mark_status_history AS h
			JOIN mark_statuses AS ms
			ON ms.mark_status_id = h.new_mark_status_id
			WHERE h.id = $1
		UNION 
			SELECT h2.id, h2.prev_id, ms2.parent_id, ms2.name  
			FROM mark_status_history AS h2
			JOIN mark_statuses AS ms2
			ON ms2.mark_status_id = h2.new_mark_status_id
			JOIN r 
			ON r.prev_id = h2.id
		)

		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			c.user_id = $2 AND mark_status_history_id IN (SELECT id FROM r)`

	if err := r.Conn.GetContext(ctx, &check, query, markStatusHistoryId, userId); err != nil {
		switch err {
		case sql.ErrNoRows:
			return check, storage.ErrNotFound
		default:
			return check, fmt.Errorf("%s: %w", op, err)
		}
	}

	return check, nil
}
