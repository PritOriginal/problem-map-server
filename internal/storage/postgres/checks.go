package postgres

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
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
				checks (user_id, mark_id, comment, result) 
			VALUES 
				(:user_id, :mark_id, :comment, :result)
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
	const op = "storage.postgres.GetReviewById"

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
		return check, fmt.Errorf("%s: %w", op, err)
	}

	return check, nil
}

func (r *ChecksRepository) GetChecksByMarkId(ctx context.Context, markId int) ([]models.Check, error) {
	const op = "storage.postgres.GetReviewsByMarkId"

	checks := []models.Check{}

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			mark_id = $1`

	if err := r.Conn.SelectContext(ctx, &checks, query, markId); err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	return checks, nil
}

func (r *ChecksRepository) GetChecksByUserId(ctx context.Context, userId int) ([]models.Check, error) {
	const op = "storage.postgres.GetReviewsByUserId"

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
