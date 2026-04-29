package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type MarksRepository struct {
	Conn *sqlx.DB
}

func NewMarks(conn *sqlx.DB) *MarksRepository {
	return &MarksRepository{Conn: conn}
}

func (repo *MarksRepository) GetMarks(ctx context.Context, filters models.GetMarksFilters) ([]models.Mark, error) {
	const op = "storage.postgres.GetMarks"

	marks := []models.Mark{}

	var conditions []string
	var args []any
	query := `
			SELECT 
				mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, created_at, updated_at 
			FROM 
				marks
			WHERE
				1=1
			`

	if len(filters.MarkStatusIds) > 0 {
		conditions = append(conditions, "mark_status_id = ANY($?)")
		args = append(args, pq.Array(filters.MarkStatusIds))
	}
	if len(filters.MarkTypeIds) > 0 {
		conditions = append(conditions, "type_mark_id = ANY($?)")
		args = append(args, pq.Array(filters.MarkTypeIds))
	}

	for i, condition := range conditions {
		query += " AND " + condition
		query = strings.Replace(query, "$?", fmt.Sprintf("$%d", len(args)-len(conditions)+i+1), 1)
	}
	if err := repo.Conn.SelectContext(ctx, &marks, query, args...); err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

func (repo *MarksRepository) GetMarkById(ctx context.Context, id int) (models.Mark, error) {
	const op = "storage.postgres.GetMarkById"

	mark := models.Mark{}

	query := `SELECT
				mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, created_at, updated_at 
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
				mark_id, description, ST_AsEWKB(geom) AS geom, type_mark_id, mark_status_id, user_id, created_at, updated_at
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
				marks (description, geom, type_mark_id, user_id) 
			VALUES 
				($1, ST_GeomFromEWKB($2), $3, $4)
			RETURNING mark_id
			`

	stmt, err := repo.Conn.PreparexContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := stmt.GetContext(ctx, &id, mark.Description, &mark.Geom, mark.MarkTypeID, mark.UserID); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (repo *MarksRepository) GetMarkTypes(ctx context.Context) ([]models.MarkType, error) {
	const op = "storage.postgres.GetMarkTypes"

	types := []models.MarkType{}

	query := "SELECT * FROM types_marks ORDER BY name"

	if err := repo.Conn.SelectContext(ctx, &types, query); err != nil {
		return types, fmt.Errorf("%s: %w", op, err)
	}

	return types, nil
}

func (repo *MarksRepository) GetMarkStatuses(ctx context.Context) ([]models.MarkStatus, error) {
	const op = "storage.postgres.GetMarkTypes"

	statuses := []models.MarkStatus{}

	query := "SELECT * FROM mark_statuses ORDER BY mark_status_id"

	if err := repo.Conn.SelectContext(ctx, &statuses, query); err != nil {
		return statuses, fmt.Errorf("%s: %w", op, err)
	}

	return statuses, nil
}

func (repo *MarksRepository) UpdateMarkStatus(ctx context.Context, markId int, markStatusId models.MarkStatusType) error {
	const op = "storage.postgres.UpdateMarkStatus"

	if _, err := repo.Conn.ExecContext(ctx, "UPDATE marks SET mark_status_id = $1 WHERE mark_id = $2", markStatusId, markId); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (repo *MarksRepository) GetMarkStatusHistoryByMarkId(ctx context.Context, markId int) ([]models.MarkStatusHistoryItem, error) {
	const op = "storage.postgres.GetMarkStatusHistoryByMarkId"

	historyItems := []models.MarkStatusHistoryItem{}

	query := `
		SELECT 
			* 
		FROM 
			mark_status_history 
		WHERE
			mark_id = $1 
		ORDER BY
			changed_at
		`

	if err := repo.Conn.SelectContext(ctx, &historyItems, query, markId); err != nil {
		return historyItems, fmt.Errorf("%s: %w", op, err)
	}

	return historyItems, nil
}

func (r *MarksRepository) GetLastMarkStatusHistoryItem(ctx context.Context, markId int) (models.MarkStatusHistoryItem, error) {
	const op = "storage.postgres.GetLastMarkStatusHistoryItemWithStatus"

	var historyItem models.MarkStatusHistoryItem

	query := `
		SELECT 
			* 
		FROM 
			mark_status_history 
		WHERE 
			mark_id = $1 
		ORDER BY 
			changed_at DESC 
		LIMIT 1
		`

	if err := r.Conn.GetContext(ctx, &historyItem, query, markId); err != nil {
		switch err {
		case sql.ErrNoRows:
			return historyItem, storage.ErrNotFound
		default:
			return historyItem, fmt.Errorf("%s: %w", op, err)
		}
	}

	return historyItem, nil
}
