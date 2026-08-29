//go:build integration

package postgres_test

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
)

func (s *PostgresSuite) TestMap_GetAdminBoundaries() {
	// Add one boundary on another admin level to exercise the filter.
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO admin_boundaries (osm_id, name, admin_level, geom) VALUES
			(1003, 'Область', 4, ST_SetSRID(ST_Multi(ST_MakeEnvelope(40, 52, 43, 54)), 4326))
	`)
	s.Require().NoError(err)

	tests := []struct {
		name      string
		filters   models.GetAdminBoundaryFilters
		wantNames []string
	}{
		{name: "all levels", wantNames: []string{"Центр", "Пустой", "Область"}},
		{name: "level 8", filters: models.GetAdminBoundaryFilters{AdminLevels: []int{8}}, wantNames: []string{"Центр", "Пустой"}},
		{name: "levels 4 and 8", filters: models.GetAdminBoundaryFilters{AdminLevels: []int{4, 8}}, wantNames: []string{"Центр", "Пустой", "Область"}},
		{name: "level without boundaries", filters: models.GetAdminBoundaryFilters{AdminLevels: []int{6}}, wantNames: []string{}},
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
