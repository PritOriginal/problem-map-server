package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/jmoiron/sqlx"
)

type MarksRepository struct {
	Conn *sqlx.DB
}

func NewMarks(conn *sqlx.DB) *MarksRepository {
	return &MarksRepository{Conn: conn}
}

func (repo *MarksRepository) GetMarks(ctx context.Context) ([]models.Mark, error) {
	const op = "storage.postgres.GetMarks"

	marks := []models.Mark{}

	query := `
			SELECT 
				mark_id, name, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, district_id, number_votes, number_checks 
			FROM 
				marks
			`

	if err := repo.Conn.SelectContext(ctx, &marks, query); err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

func (repo *MarksRepository) GetMarkById(ctx context.Context, id int) (models.Mark, error) {
	const op = "storage.postgres.GetMarkById"

	mark := models.Mark{}

	query := `SELECT
				mark_id, name, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, district_id, number_votes, number_checks 
			FROM 
				marks 
			WHERE 
				mark_id = $1
			`

	if err := repo.Conn.GetContext(ctx, &mark, query, id); err != nil {
		switch err {
		case sql.ErrNoRows:
			return mark, storage.ErrNotFound
		default:
			return mark, fmt.Errorf("%s: %w", op, err)
		}
	}

	return mark, nil
}

func (repo *MarksRepository) GetMarksByUserId(ctx context.Context, userId int) ([]models.Mark, error) {
	const op = "storage.postgres.GetMarksByUserId"

	marks := []models.Mark{}

	query := `SELECT
				mark_id, name, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, district_id, number_votes, number_checks 
			FROM 
				marks 
			WHERE 
				user_id = $1
			`

	if err := repo.Conn.SelectContext(ctx, &marks, query, userId); err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

func (repo *MarksRepository) AddMark(ctx context.Context, mark models.Mark) (int64, error) {
	const op = "storage.postgres.AddMark"

	var id int64

	query := `
			INSERT INTO 
				marks (name, geom, type_mark_id, user_id, district_id) 
			VALUES 
				($1, ST_GeomFromEWKB($2), $3, $4, $5)
			RETURNING mark_id
			`

	stmt, err := repo.Conn.PreparexContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := stmt.GetContext(ctx, &id, mark.Name, &mark.Geom, mark.TypeMarkID, mark.UserID, mark.DistrictID); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (repo *MarksRepository) GetMarkTypes(ctx context.Context) ([]models.MarkType, error) {
	const op = "storage.postgres.GetMarkTypes"

	types := []models.MarkType{}

	query := "SELECT * FROM types_marks"

	if err := repo.Conn.SelectContext(ctx, &types, query); err != nil {
		return types, fmt.Errorf("%s: %w", op, err)
	}

	return types, nil
}

func (repo *MarksRepository) GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error) {
	const op = "storage.postgres.GetMarkTypes"

	statuses := []models.MarkStatus{}

	query := "SELECT * FROM mark_statuses"

	if err := repo.Conn.SelectContext(ctx, &statuses, query); err != nil {
		return statuses, fmt.Errorf("%s: %w", op, err)
	}

	return statuses, nil
}
