package postgres

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/jmoiron/sqlx"
)

type MapRepository interface {
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
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
	const op = "storage.postgres.GetRegions"

	regions := []models.Region{}

	query := "SELECT name, ST_AsEWKB(geom) AS geom FROM regions"
	if err := repo.Conn.SelectContext(ctx, &regions, query); err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}

	return regions, nil
}

func (repo *MapRepo) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "storage.postgres.GetCities"

	cities := []models.City{}

	query := "SELECT name, ST_AsEWKB(geom) AS geom FROM cities"
	if err := repo.Conn.SelectContext(ctx, &cities, query); err != nil {
		return cities, fmt.Errorf("%s: %w", op, err)
	}

	return cities, nil
}

func (repo *MapRepo) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "storage.postgres.GetDistricts"

	districts := []models.District{}

	query := "SELECT district_id, name, ST_AsEWKB(geom) AS geom FROM districts"
	if err := repo.Conn.SelectContext(ctx, &districts, query); err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}

	return districts, nil
}

func (repo *MapRepo) GetMarks(ctx context.Context) ([]models.Mark, error) {
	const op = "storage.postgres.GetMarks"

	marks := []models.Mark{}

	query := `
			SELECT 
				mark_id, name, ST_AsEWKB(geom) AS geom, type_mark_id, user_id, district_id, number_votes, number_checks 
			FROM 
				marks
			`

	if err := repo.Conn.SelectContext(ctx, &marks, query); err != nil {
		return marks, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

func (repo *MapRepo) AddMark(ctx context.Context, mark models.Mark) (int64, error) {
	const op = "storage.postgres.GetMarks"

	var id int64

	query := `
			INSERT INTO 
				marks (name, geom, type_mark_id, user_id, district_id, number_votes, number_checks) 
			VALUES 
				($1, ST_GeomFromEWKB($2), $3, $4, $5, $6, $7)
			RETURNING mark_id
			`

	stmt, err := repo.Conn.PreparexContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := stmt.GetContext(ctx, &id, mark.Name, &mark.Geom, mark.TypeMarkID, mark.UserID, mark.DistrictID, mark.NumberVotes, mark.NumberChecks); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}
