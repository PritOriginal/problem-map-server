//go:build integration

package postgres_test

import (
	"math"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

func (s *PostgresSuite) TestMap_GetAdminBoundaries() {
	// Add one boundary on another admin level to exercise the filter.
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO admin_boundaries (osm_id, name, admin_level, geom) VALUES
			(1003, 'Область', 4, ST_SetSRID(ST_Multi(ST_MakeEnvelope(40, 52, 43, 54)), 4326))
	`)
	s.Require().NoError(err)

	// WithGeometry is what the caller asks for, and the handler defaults it to true
	// (see handler/map/dto.go): the cases below therefore set it explicitly rather
	// than leaning on the zero value, which means "id, name and admin_level only".
	tests := []struct {
		name      string
		filters   models.GetAdminBoundaryFilters
		wantNames []string
	}{
		{name: "all levels", filters: models.GetAdminBoundaryFilters{WithGeometry: true}, wantNames: []string{"Центр", "Пустой", "Область"}},
		{name: "level 8", filters: models.GetAdminBoundaryFilters{AdminLevels: []int{8}, WithGeometry: true}, wantNames: []string{"Центр", "Пустой"}},
		{name: "levels 4 and 8", filters: models.GetAdminBoundaryFilters{AdminLevels: []int{4, 8}, WithGeometry: true}, wantNames: []string{"Центр", "Пустой", "Область"}},
		{name: "level without boundaries", filters: models.GetAdminBoundaryFilters{AdminLevels: []int{6}, WithGeometry: true}, wantNames: []string{}},
		// The index the map asks for on load: the geometry dominates the response,
		// so it is left out and every other column still has to arrive.
		{name: "without geometry", filters: models.GetAdminBoundaryFilters{}, wantNames: []string{"Центр", "Пустой", "Область"}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			boundaries, err := s.maps.GetAdminBoundaries(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.NotNil(boundaries)

			names := make([]string, 0, len(boundaries))
			for _, b := range boundaries {
				names = append(names, b.Name)
				s.NotZero(b.Id)
				s.NotZero(b.AdminLevel)
				if !tt.filters.WithGeometry {
					s.Nil(b.Geom)
					continue
				}
				s.Require().NotNil(b.Geom)
				s.Equal(4326, b.Geom.Ewkb.SRID())
				s.Equal(1, b.Geom.Ewkb.NumPolygons())
			}
			s.ElementsMatch(tt.wantNames, names)
		})
	}
}

func (s *PostgresSuite) TestMap_GetAdminBoundariesMarksCount() {
	// Add a closed and a rediscovered mark inside "Центр" so every counter is
	// exercised: statuses 1 (mark1), 2 (mark2), 4 and 5 (added here).
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO marks (description, geom, type_mark_id, user_id) VALUES
			('Закрытая', ST_SetSRID(ST_MakePoint(41.40, 52.70), 4326), 1, 1),
			('Переоткрытая', ST_SetSRID(ST_MakePoint(41.41, 52.70), 4326), 3, 2)
	`)
	s.Require().NoError(err)
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, 4, models.ClosedStatus))
	s.Require().NoError(s.marks.UpdateMarkStatus(s.ctx, 5, models.RediscoveredStatus))

	all := models.AdminBoundaryMarksCount{
		Id: fxBoundaryMain, Name: "Центр",
		TotalCount: 4, UnconfirmedCount: 1, ConfirmedCount: 2, UnderReviewCount: 0, ClosedCount: 1,
	}
	void := models.AdminBoundaryMarksCount{Id: fxBoundaryVoid, Name: "Пустой"}

	tests := []struct {
		name    string
		filters models.GetAdminBoundaryMarksCountFilters
		want    []models.AdminBoundaryMarksCount
	}{
		{
			name: "no filters: boundary without marks is kept with zero counts",
			want: []models.AdminBoundaryMarksCount{all, void},
		},
		{
			name:    "type filter narrows the counts but keeps every boundary",
			filters: models.GetAdminBoundaryMarksCountFilters{MarkTypeIds: []int{1}},
			want: []models.AdminBoundaryMarksCount{
				{Id: fxBoundaryMain, Name: "Центр", TotalCount: 2, UnconfirmedCount: 1, ClosedCount: 1},
				void,
			},
		},
		{
			name:    "type without marks inside gives zero rows for each boundary",
			filters: models.GetAdminBoundaryMarksCountFilters{MarkTypeIds: []int{4}},
			want: []models.AdminBoundaryMarksCount{
				{Id: fxBoundaryMain, Name: "Центр"},
				void,
			},
		},
		{
			name:    "admin level filter drops boundaries",
			filters: models.GetAdminBoundaryMarksCountFilters{AdminLevels: []int{8}},
			want:    []models.AdminBoundaryMarksCount{all, void},
		},
		{
			name:    "admin level without boundaries",
			filters: models.GetAdminBoundaryMarksCountFilters{AdminLevels: []int{4}},
			want:    []models.AdminBoundaryMarksCount{},
		},
		{
			name:    "both filters",
			filters: models.GetAdminBoundaryMarksCountFilters{AdminLevels: []int{8}, MarkTypeIds: []int{2, 3}},
			want: []models.AdminBoundaryMarksCount{
				{Id: fxBoundaryMain, Name: "Центр", TotalCount: 2, ConfirmedCount: 2},
				void,
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.maps.GetAdminBoundariesMarksCount(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.NotNil(got)
			s.Equal(tt.want, got, "rows must be ordered by boundary id")
		})
	}
}

func (s *PostgresSuite) TestMap_GetAdminBoundariesMarksCount_StatusAndDateFilters() {
	void := models.AdminBoundaryMarksCount{Id: fxBoundaryVoid, Name: "Пустой"}

	tests := []struct {
		name    string
		filters models.GetAdminBoundaryMarksCountFilters
		want    models.AdminBoundaryMarksCount
	}{
		{
			name:    "status filter keeps only confirmed",
			filters: models.GetAdminBoundaryMarksCountFilters{MarkStatusIds: []int{int(models.ConfirmedStatus)}},
			want:    models.AdminBoundaryMarksCount{Id: fxBoundaryMain, Name: "Центр", TotalCount: 1, ConfirmedCount: 1},
		},
		{
			name:    "several statuses",
			filters: models.GetAdminBoundaryMarksCountFilters{MarkStatusIds: []int{int(models.UnconfirmedStatus), int(models.ConfirmedStatus)}},
			want:    models.AdminBoundaryMarksCount{Id: fxBoundaryMain, Name: "Центр", TotalCount: 2, UnconfirmedCount: 1, ConfirmedCount: 1},
		},
		{
			name:    "from excludes the 40 days old mark",
			filters: models.GetAdminBoundaryMarksCountFilters{DateRange: models.DateRange{From: s.daysAgo(20)}},
			want:    models.AdminBoundaryMarksCount{Id: fxBoundaryMain, Name: "Центр", TotalCount: 1, ConfirmedCount: 1},
		},
		{
			name:    "to excludes the recent mark",
			filters: models.GetAdminBoundaryMarksCountFilters{DateRange: models.DateRange{To: s.daysAgo(20)}},
			want:    models.AdminBoundaryMarksCount{Id: fxBoundaryMain, Name: "Центр", TotalCount: 1, UnconfirmedCount: 1},
		},
		{
			name: "status and range combined to nothing",
			filters: models.GetAdminBoundaryMarksCountFilters{
				MarkStatusIds: []int{int(models.ConfirmedStatus)},
				DateRange:     models.DateRange{To: s.daysAgo(20)},
			},
			want: models.AdminBoundaryMarksCount{Id: fxBoundaryMain, Name: "Центр"},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.maps.GetAdminBoundariesMarksCount(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.Equal([]models.AdminBoundaryMarksCount{tt.want, void}, got)
		})
	}
}

func (s *PostgresSuite) TestMap_GetHeatmap() {
	// Bbox of the "Центр" boundary: marks 1 (type 1, unconfirmed) and
	// 2 (type 2, confirmed) are inside, ~700 m apart; mark 3 is outside.
	bbox := models.BBox{MinLon: 41.39, MinLat: 52.69, MaxLon: 41.42, MaxLat: 52.71}

	tests := []struct {
		name      string
		filters   models.HeatmapFilters
		wantCells int
		wantTotal int
	}{
		{
			name:      "coarse grid: both marks, cell count between 1 and 2",
			filters:   models.HeatmapFilters{BBox: bbox, CellM: 5000},
			wantCells: -1,
			wantTotal: 2,
		},
		{
			name:      "fine grid: one mark per cell",
			filters:   models.HeatmapFilters{BBox: bbox, CellM: 100},
			wantCells: 2,
			wantTotal: 2,
		},
		{
			name:      "type filter",
			filters:   models.HeatmapFilters{BBox: bbox, CellM: 100, MarkTypeIds: []int{2}},
			wantCells: 1,
			wantTotal: 1,
		},
		{
			name:      "status filter",
			filters:   models.HeatmapFilters{BBox: bbox, CellM: 100, MarkStatusIds: []int{int(models.UnconfirmedStatus)}},
			wantCells: 1,
			wantTotal: 1,
		},
		{
			name:      "type and status that never match together",
			filters:   models.HeatmapFilters{BBox: bbox, CellM: 100, MarkTypeIds: []int{2}, MarkStatusIds: []int{int(models.UnconfirmedStatus)}},
			wantCells: 0,
		},
		{
			name:      "bbox without marks",
			filters:   models.HeatmapFilters{BBox: models.BBox{MinLon: 41.50, MinLat: 52.80, MaxLon: 41.52, MaxLat: 52.82}, CellM: 100},
			wantCells: 0,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			cells, err := s.maps.GetHeatmap(s.ctx, tt.filters)
			s.Require().NoError(err)
			s.NotNil(cells)
			if tt.wantCells >= 0 {
				s.Len(cells, tt.wantCells)
			} else {
				s.GreaterOrEqual(len(cells), 1)
				s.LessOrEqual(len(cells), 2)
			}

			total := 0
			for _, cell := range cells {
				total += cell.Count
				s.Positive(cell.Count)
				s.Require().NotNil(cell.Geom)
				s.Equal(4326, cell.Geom.Ewkb.SRID())
				// A hexagon ring: 6 vertices plus the closing point.
				s.Equal(7, cell.Geom.Ewkb.NumCoords())
				// The cell overlaps the bbox (it may stick out along the edge).
				bounds := cell.Geom.Ewkb.Bounds()
				s.Less(bounds.Min(0), tt.filters.BBox.MaxLon)
				s.Greater(bounds.Max(0), tt.filters.BBox.MinLon)
				s.Less(bounds.Min(1), tt.filters.BBox.MaxLat)
				s.Greater(bounds.Max(1), tt.filters.BBox.MinLat)
			}
			s.Equal(tt.wantTotal, total)
		})
	}
}

// TestMap_GetHeatmap_CellIsGroundMeters checks that cell_m is a ground size:
// at 52.7 N a flat-top hexagon of 250 m (center-to-vertex) spans ~500 m
// east-west and ~433 m north-south on the ground, not 500 EPSG:3857 units
// (which would be ~300 m here).
func (s *PostgresSuite) TestMap_GetHeatmap_CellIsGroundMeters() {
	bbox := models.BBox{MinLon: 41.39, MinLat: 52.69, MaxLon: 41.42, MaxLat: 52.71}
	cells, err := s.maps.GetHeatmap(s.ctx, models.HeatmapFilters{BBox: bbox, CellM: 250})
	s.Require().NoError(err)
	// Marks 1 and 2 are ~850 m apart, so they land in different cells.
	s.Require().Len(cells, 2)

	const metersPerDegree = 111_319.49
	for _, cell := range cells {
		bounds := cell.Geom.Ewkb.Bounds()
		midLat := (bounds.Min(1) + bounds.Max(1)) / 2
		widthM := (bounds.Max(0) - bounds.Min(0)) * metersPerDegree * math.Cos(midLat*math.Pi/180)
		heightM := (bounds.Max(1) - bounds.Min(1)) * metersPerDegree
		s.InDelta(2*250, widthM, 25)
		s.InDelta(math.Sqrt(3)*250, heightM, 25)
	}
}

func (s *PostgresSuite) TestMap_GetAdminBoundariesMarksCount_NoMarksAtAll() {
	_, err := s.db.ExecContext(s.ctx, `TRUNCATE TABLE checks, tasks, mark_status_history, marks RESTART IDENTITY CASCADE`)
	s.Require().NoError(err)

	got, err := s.maps.GetAdminBoundariesMarksCount(s.ctx, models.GetAdminBoundaryMarksCountFilters{})
	s.Require().NoError(err)
	s.Equal([]models.AdminBoundaryMarksCount{
		{Id: fxBoundaryMain, Name: "Центр"},
		{Id: fxBoundaryVoid, Name: "Пустой"},
	}, got)
}

func (s *PostgresSuite) TestMap_RegionsCitiesDistricts() {
	s.Run("empty tables return empty slices", func() {
		regions, err := s.maps.GetRegions(s.ctx)
		s.Require().NoError(err)
		s.NotNil(regions)
		s.Empty(regions)

		cities, err := s.maps.GetCities(s.ctx)
		s.Require().NoError(err)
		s.NotNil(cities)
		s.Empty(cities)

		districts, err := s.maps.GetDistricts(s.ctx)
		s.Require().NoError(err)
		s.NotNil(districts)
		s.Empty(districts)
	})

	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO regions (name, geom) VALUES ('Регион', ST_SetSRID(ST_MakeEnvelope(40, 52, 43, 54), 4326));
		INSERT INTO cities (name, region_id, geom) VALUES ('Город', 1, ST_SetSRID(ST_MakeEnvelope(41, 52.5, 42, 53), 4326));
		INSERT INTO districts (name, city_id, geom) VALUES
			('Район 1', 1, ST_SetSRID(ST_MakeEnvelope(41, 52.5, 41.5, 53), 4326)),
			('Район 2', 1, ST_SetSRID(ST_MakeEnvelope(41.5, 52.5, 42, 53), 4326));
	`)
	s.Require().NoError(err)

	s.Run("regions are scanned with polygon geometry", func() {
		regions, err := s.maps.GetRegions(s.ctx)
		s.Require().NoError(err)
		s.Require().Len(regions, 1)
		s.Equal("Регион", regions[0].Name)
		s.Require().NotNil(regions[0].Geom)
		s.Equal(4326, regions[0].Geom.Ewkb.SRID())
		s.Equal(5, regions[0].Geom.Ewkb.NumCoords())
	})

	s.Run("cities", func() {
		cities, err := s.maps.GetCities(s.ctx)
		s.Require().NoError(err)
		s.Require().Len(cities, 1)
		s.Equal("Город", cities[0].Name)
		s.Require().NotNil(cities[0].Geom)
	})

	s.Run("districts", func() {
		districts, err := s.maps.GetDistricts(s.ctx)
		s.Require().NoError(err)
		s.Require().Len(districts, 2)
		s.Equal([]int{1, 2}, []int{districts[0].ID, districts[1].ID})
		s.Equal("Район 1", districts[0].Name)
		s.Require().NotNil(districts[1].Geom)
	})
}

func (s *PostgresSuite) TestMap_GetAdminBoundaryById() {
	boundary, err := s.maps.GetAdminBoundaryById(s.ctx, fxBoundaryMain)
	s.Require().NoError(err)
	s.Equal(fxBoundaryMain, boundary.Id)
	s.Equal("Центр", boundary.Name)
	s.Equal(8, boundary.AdminLevel)
	s.Require().NotNil(boundary.Geom)
	s.Equal(1, boundary.Geom.Ewkb.NumPolygons())

	_, err = s.maps.GetAdminBoundaryById(s.ctx, 404)
	s.ErrorIs(err, repository.ErrNotFound)
}
