package db

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/jmoiron/sqlx"
)

type MapRepository interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
	GetMarks(ctx context.Context) ([]models.Mark, error)
	AddMark(ctx context.Context, mark models.Mark) error
}

type MapRepo struct {
	Conn *sqlx.DB
}

func NewMap(conn *sqlx.DB) *MapRepo {
	return &MapRepo{Conn: conn}
}

func (repo *MapRepo) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "storage.db.GetRegions"

	var regions []models.Region

	query := "SELECT name, ST_AsEWKB(geom) FROM regions"
	if err := repo.Conn.SelectContext(ctx, &regions, query); err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}

	return regions, nil
}

func (repo *MapRepo) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "storage.db.GetDistricts"

	var districts []models.District

	query := "SELECT district_id, name, ST_AsEWKB(geom) AS geom FROM districts"
	if err := repo.Conn.SelectContext(ctx, &districts, query); err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}

	return districts, nil
}

func (repo *MapRepo) GetMarks(ctx context.Context) ([]models.Mark, error) {
	const op = "storage.db.GetMarks"

	var marks []models.Mark

	query := `SELECT 
				mark_id, name, ST_AsEWKB(geom) AS geom, type_mark_id, user_id, district_id, number_votes, number_checks 
			FROM marks`
	if err := repo.Conn.SelectContext(ctx, &marks, query); err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

func (repo *MapRepo) AddMark(ctx context.Context, mark models.Mark) error {
	const op = "storage.db.GetMarks"

	query := `INSERT INTO 
				marks (name, geom, type_mark_id, user_id, district_id, number_votes, number_checks) 
			VALUES 
				(:name, :geom, :type_mark_id, :user_id, :district_id, :number_votes, :number_checks)`

	if _, err := repo.Conn.NamedExecContext(ctx, query, mark); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (repo *MapRepo) AddPhotos(ctx context.Context) error {
	// const op = "storage.db.AddPhotos"

	// query := ""

	return nil
}
