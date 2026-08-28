package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type MapRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewMap(db *sqlx.DB, c *trmsqlx.CtxGetter) *MapRepository {
	return &MapRepository{
		db:     db,
		getter: c,
	}
}

func (r *MapRepository) GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error) {
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

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &boundaries, query, args...); err != nil {
		return boundaries, fmt.Errorf("%s: %w", op, err)
	}

	return boundaries, nil
}

func (r *MapRepository) GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error) {
	const op = "storage.postgres.GetAdminBoundariesMarksCount"

	boundariesCount := []models.AdminBoundaryMarksCount{}
	var args []any

	// Conditions on marks live in the LEFT JOIN ... ON clause so that
	// boundaries without matching marks are kept (with zero counts);
	// conditions on the boundaries themselves live in WHERE.
	joinConditions := []string{"ST_Contains(b.geom, m.geom)"}
	whereConditions := []string{"1=1"}

	if len(filters.AdminLevels) > 0 {
		args = append(args, pq.Array(filters.AdminLevels))
		whereConditions = append(whereConditions, fmt.Sprintf("b.admin_level = ANY($%d)", len(args)))
	}
	if len(filters.MarkTypeIds) > 0 {
		args = append(args, pq.Array(filters.MarkTypeIds))
		joinConditions = append(joinConditions, fmt.Sprintf("m.type_mark_id = ANY($%d)", len(args)))
	}

	query := fmt.Sprintf(`
		SELECT
			b.id AS boundary_id,
			b.name AS boundary_name,
			COUNT(m.mark_id) AS total_count,
			COUNT(m.mark_id) FILTER (WHERE m.mark_status_id = 1) AS unconfirmed_count,
			COUNT(m.mark_id) FILTER (WHERE m.mark_status_id IN (2,4)) AS confirmed_count,
			COUNT(m.mark_id) FILTER (WHERE m.mark_status_id = 3) AS under_review_count,
			COUNT(m.mark_id) FILTER (WHERE m.mark_status_id = 5) AS closed_count
		FROM
			admin_boundaries b
		LEFT JOIN
			marks m ON %s
		WHERE
			%s
		GROUP BY
			b.id, b.name
		ORDER BY
			b.id
	`, strings.Join(joinConditions, " AND "), strings.Join(whereConditions, " AND "))

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &boundariesCount, query, args...); err != nil {
		return boundariesCount, fmt.Errorf("%s: %w", op, err)
	}

	return boundariesCount, nil
}

func (r *MapRepository) GetRegions(ctx context.Context) ([]models.Region, error) {
	const op = "storage.postgres.GetRegions"

	regions := []models.Region{}

	query := "SELECT name, ST_AsEWKB(geom) AS geom FROM regions"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &regions, query); err != nil {
		return regions, fmt.Errorf("%s: %w", op, err)
	}

	return regions, nil
}

func (r *MapRepository) GetCities(ctx context.Context) ([]models.City, error) {
	const op = "storage.postgres.GetCities"

	cities := []models.City{}

	query := "SELECT name, ST_AsEWKB(geom) AS geom FROM cities"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &cities, query); err != nil {
		return cities, fmt.Errorf("%s: %w", op, err)
	}

	return cities, nil
}

func (r *MapRepository) GetDistricts(ctx context.Context) ([]models.District, error) {
	const op = "storage.postgres.GetDistricts"

	districts := []models.District{}

	query := "SELECT district_id, name, ST_AsEWKB(geom) AS geom FROM districts"
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &districts, query); err != nil {
		return districts, fmt.Errorf("%s: %w", op, err)
	}

	return districts, nil
}
