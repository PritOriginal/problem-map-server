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

	q := newListQuery("id, name, admin_level, ST_AsEWKB(geom) AS geom", "admin_boundaries").
		OrderBy("id ASC")

	if len(filters.AdminLevels) > 0 {
		q.Where("admin_level IN (?)", filters.AdminLevels)
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.AdminBoundary](ctx, tr, q)
	if err != nil {
		return page.Items, fmt.Errorf("%s: %w", op, err)
	}

	return page.Items, nil
}

func (r *MapRepository) GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error) {
	const op = "storage.postgres.GetAdminBoundariesMarksCount"

	boundariesCount := []models.AdminBoundaryMarksCount{}
	var args []any

	// Conditions on marks live in the LEFT JOIN ... ON clause so that
	// boundaries without matching marks are kept (with zero counts);
	// conditions on the boundaries themselves live in WHERE.
	joinConditions := []string{"ST_Contains(b.geom, m.geom)"}
	whereConditions := []string{"TRUE"}

	if len(filters.AdminLevels) > 0 {
		args = append(args, pq.Array(filters.AdminLevels))
		whereConditions = append(whereConditions, fmt.Sprintf("b.admin_level = ANY($%d)", len(args)))
	}
	if len(filters.MarkTypeIds) > 0 {
		args = append(args, pq.Array(filters.MarkTypeIds))
		joinConditions = append(joinConditions, fmt.Sprintf("m.type_mark_id = ANY($%d)", len(args)))
	}
	if len(filters.MarkStatusIds) > 0 {
		args = append(args, pq.Array(filters.MarkStatusIds))
		joinConditions = append(joinConditions, fmt.Sprintf("m.mark_status_id = ANY($%d)", len(args)))
	}
	if !filters.From.IsZero() {
		args = append(args, filters.From)
		joinConditions = append(joinConditions, fmt.Sprintf("m.created_at >= $%d", len(args)))
	}
	if !filters.To.IsZero() {
		args = append(args, filters.To)
		joinConditions = append(joinConditions, fmt.Sprintf("m.created_at <= $%d", len(args)))
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

// GetHeatmap bins the marks inside the bbox into a hexagonal grid built in
// EPSG:3857 (ST_HexagonGrid / ST_Hexagon, PostGIS >= 3.1) with cells of CellM
// ground meters (see HeatmapFilters.CellSize3857) and returns the non-empty
// cells back in WGS84.
//
// Each mark looks up its own cell in the few hexagons around it (the grid is
// anchored at the SRS origin, so cell indices are global), which is O(marks)
// instead of the O(marks x cells) join of the whole grid against the points.
// A mark on an edge is assigned to the lowest (i, j) of the touching cells,
// so it is counted exactly once.
func (r *MapRepository) GetHeatmap(ctx context.Context, filters models.HeatmapFilters) ([]models.HeatmapCell, error) {
	const op = "storage.postgres.GetHeatmap"

	b := filters.BBox
	args := []any{b.MinLon, b.MinLat, b.MaxLon, b.MaxLat, filters.CellSize3857()}
	conds := []string{"ST_Intersects(m.geom, ST_MakeEnvelope($1, $2, $3, $4, 4326))"}

	if len(filters.MarkTypeIds) > 0 {
		args = append(args, pq.Array(filters.MarkTypeIds))
		conds = append(conds, fmt.Sprintf("m.type_mark_id = ANY($%d)", len(args)))
	}
	if len(filters.MarkStatusIds) > 0 {
		args = append(args, pq.Array(filters.MarkStatusIds))
		conds = append(conds, fmt.Sprintf("m.mark_status_id = ANY($%d)", len(args)))
	}

	query := fmt.Sprintf(`
		WITH pts AS (
			SELECT ST_Transform(m.geom, 3857) AS geom
			FROM marks m
			WHERE %s
		),
		cells AS (
			SELECT hex.i, hex.j, COUNT(*) AS count
			FROM pts
			CROSS JOIN LATERAL (
				SELECT g.i, g.j
				FROM ST_HexagonGrid($5, ST_Expand(pts.geom, $5)) AS g
				WHERE ST_Intersects(g.geom, pts.geom)
				ORDER BY g.i, g.j
				LIMIT 1
			) AS hex
			GROUP BY hex.i, hex.j
		)
		SELECT
			ST_AsEWKB(ST_Transform(ST_SetSRID(ST_Hexagon($5, i, j), 3857), 4326)) AS geom,
			count
		FROM cells
		ORDER BY i, j
	`, strings.Join(conds, " AND "))

	cells := []models.HeatmapCell{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &cells, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return cells, nil
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
