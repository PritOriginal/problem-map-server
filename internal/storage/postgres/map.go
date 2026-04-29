package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type MapRepository struct {
	Conn *sqlx.DB
}

func NewMap(conn *sqlx.DB) *MapRepository {
	return &MapRepository{Conn: conn}
}

func (repo *MapRepository) GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error) {
	const op = "storage.postgres.GetAdminBoundaries"

	boundaries := []models.AdminBoundary{}
	var conditions []string
	var args []any

	query := "SELECT id, name, admin_level, ST_AsEWKB(geom) AS geom FROM admin_boundaries WHERE 1=1"

	if len(filters.AdminLevels) > 0 {
		conditions = append(conditions, "admin_level = ANY($?)")
		args = append(args, pq.Array(filters.AdminLevels))
	}

	for i, condition := range conditions {
		query += " AND " + condition
		query = strings.Replace(query, "$?", fmt.Sprintf("$%d", len(args)-len(conditions)+i+1), 1)
	}

	if err := repo.Conn.SelectContext(ctx, &boundaries, query, args...); err != nil {
		return boundaries, fmt.Errorf("%s: %w", op, err)
	}

	return boundaries, nil
}

func (repo *MapRepository) GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error) {
	const op = "storage.postgres.GetAdminBoundariesMarksCount"

	boundariesCount := []models.AdminBoundaryMarksCount{}
	var conditions []string
	var args []any

	query :=
		`
		SELECT
			b.id AS boundary_id,
			b.name AS boundary_name,
			COUNT(m.*) AS total_count,
			COUNT(*) FILTER (WHERE m.mark_status_id = 1) AS unconfirmed_count,
			COUNT(*) FILTER (WHERE m.mark_status_id IN (2,4)) AS confirmed_count,
			COUNT(*) FILTER (WHERE m.mark_status_id = 3) AS under_review_count,
			COUNT(*) FILTER (WHERE m.mark_status_id = 5) AS closed_count
		FROM
			admin_boundaries b
		LEFT JOIN
			marks m ON ST_Contains(b.geom, m.geom)
		WHERE 
			1=1	
		GROUP BY
			b.id, b.name
		ORDER BY
			b.id;
	`

	if len(filters.AdminLevels) > 0 {
		conditions = append(conditions, "admin_level = ANY($?)")
		args = append(args, pq.Array(filters.AdminLevels))
	}
	if len(filters.MarkTypeIds) > 0 {
		conditions = append(conditions, "type_mark_id = ANY($?)")
		args = append(args, pq.Array(filters.MarkTypeIds))
	}

	whereQuery := ""
	for i, condition := range conditions {
		whereQuery += " AND " + condition
		whereQuery = strings.Replace(whereQuery, "$?", fmt.Sprintf("$%d", len(args)-len(conditions)+i+1), 1)
	}
	if whereQuery != "" {
		query = strings.Replace(query, "1=1", "1=1"+whereQuery, 1)
	}

	if err := repo.Conn.SelectContext(ctx, &boundariesCount, query, args...); err != nil {
		return boundariesCount, fmt.Errorf("%s: %w", op, err)
	}

	return boundariesCount, nil
}

func (repo *MapRepository) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "storage.postgres.GetRegions"

	regions := []models.Region{}

	query := "SELECT name, ST_AsEWKB(geom) AS geom FROM regions"
	if err := repo.Conn.SelectContext(ctx, &regions, query); err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}

	return regions, nil
}

func (repo *MapRepository) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "storage.postgres.GetCities"

	cities := []models.City{}

	query := "SELECT name, ST_AsEWKB(geom) AS geom FROM cities"
	if err := repo.Conn.SelectContext(ctx, &cities, query); err != nil {
		return cities, fmt.Errorf("%s: %w", op, err)
	}

	return cities, nil
}

func (repo *MapRepository) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "storage.postgres.GetDistricts"

	districts := []models.District{}

	query := "SELECT district_id, name, ST_AsEWKB(geom) AS geom FROM districts"
	if err := repo.Conn.SelectContext(ctx, &districts, query); err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}

	return districts, nil
}
