package maprest

import (
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
)

// MsgTooManyCells is the 400 message when the heatmap grid is too fine.
const MsgTooManyCells = "too many cells: increase cell_m"

type GetAdminBoundariesResponse struct {
	AdminBoundaries []models.AdminBoundary `json:"admin_boundaries"`
}

// GetAdminBoundariesMarksCountRequest is bound from the query string of
// GET /map/admin-boundaries/marks/count.
type GetAdminBoundariesMarksCountRequest struct {
	AdminLevels   string `form:"admin_levels"`
	MarkTypeIds   string `form:"mark_type_ids"`
	MarkStatusIds string `form:"mark_status_ids"`
	// From / To are RFC3339 timestamps bounding marks' creation.
	From string `form:"from"`
	To   string `form:"to"`
}

// Filters converts the request to domain filters. Returned errors are safe
// to show to the client.
func (r GetAdminBoundariesMarksCountRequest) Filters() (models.GetAdminBoundaryMarksCountFilters, error) {
	adminLevels, err := handlers.ParseIntArray(r.AdminLevels)
	if err != nil {
		return models.GetAdminBoundaryMarksCountFilters{}, fmt.Errorf("failed parse admin levels")
	}
	markTypeIds, err := handlers.ParseIntArray(r.MarkTypeIds)
	if err != nil {
		return models.GetAdminBoundaryMarksCountFilters{}, fmt.Errorf("failed parse mark type ids")
	}
	markStatusIds, err := handlers.ParseIntArray(r.MarkStatusIds)
	if err != nil {
		return models.GetAdminBoundaryMarksCountFilters{}, fmt.Errorf("failed parse mark status ids")
	}
	dates, err := parseDateRange(r.From, r.To)
	if err != nil {
		return models.GetAdminBoundaryMarksCountFilters{}, err
	}
	return models.GetAdminBoundaryMarksCountFilters{
		AdminLevels:   adminLevels,
		MarkTypeIds:   markTypeIds,
		MarkStatusIds: markStatusIds,
		DateRange:     dates,
	}, nil
}

// parseDateRange parses optional RFC3339 bounds.
func parseDateRange(from, to string) (models.DateRange, error) {
	var r models.DateRange
	var err error
	if from != "" {
		if r.From, err = time.Parse(time.RFC3339, from); err != nil {
			return models.DateRange{}, fmt.Errorf("from must be RFC3339")
		}
	}
	if to != "" {
		if r.To, err = time.Parse(time.RFC3339, to); err != nil {
			return models.DateRange{}, fmt.Errorf("to must be RFC3339")
		}
	}
	return r, nil
}

type GetAdminBoundariesMarksCountResponse struct {
	AdminBoundaries []models.AdminBoundaryMarksCount `json:"admin_boundaries"`
}

// GetHeatmapRequest is bound from the query string of GET /map/heatmap.
type GetHeatmapRequest struct {
	// BBox is "minLon,minLat,maxLon,maxLat".
	BBox          string  `form:"bbox" binding:"required"`
	CellM         float64 `form:"cell_m" binding:"omitempty,gt=0"`
	MarkTypeIds   string  `form:"mark_type_ids"`
	MarkStatusIds string  `form:"mark_status_ids"`
}

// Filters converts the request to domain filters. Returned errors are safe
// to show to the client.
func (r GetHeatmapRequest) Filters() (models.HeatmapFilters, error) {
	bbox, err := models.ParseBBox(r.BBox)
	if err != nil {
		return models.HeatmapFilters{}, err
	}
	markTypeIds, err := handlers.ParseIntArray(r.MarkTypeIds)
	if err != nil {
		return models.HeatmapFilters{}, fmt.Errorf("failed parse mark type ids")
	}
	markStatusIds, err := handlers.ParseIntArray(r.MarkStatusIds)
	if err != nil {
		return models.HeatmapFilters{}, fmt.Errorf("failed parse mark status ids")
	}
	return models.HeatmapFilters{
		BBox:          bbox,
		CellM:         r.CellM,
		MarkTypeIds:   markTypeIds,
		MarkStatusIds: markStatusIds,
	}, nil
}

// HeatmapResponse is a GeoJSON FeatureCollection of hexagons.
type HeatmapResponse struct {
	Type     string           `json:"type" enums:"FeatureCollection"`
	Features []HeatmapFeature `json:"features"`
}

// HeatmapFeature is a GeoJSON Feature with a polygon geometry and the
// number of marks inside it.
type HeatmapFeature struct {
	Type       string            `json:"type" enums:"Feature"`
	Geometry   *models.Polygon   `json:"geometry"`
	Properties HeatmapProperties `json:"properties"`
}

type HeatmapProperties struct {
	Count int `json:"count"`
}

// NewHeatmapResponse wraps the cells into a FeatureCollection.
func NewHeatmapResponse(cells []models.HeatmapCell) HeatmapResponse {
	features := make([]HeatmapFeature, 0, len(cells))
	for _, cell := range cells {
		features = append(features, HeatmapFeature{
			Type:       "Feature",
			Geometry:   cell.Geom,
			Properties: HeatmapProperties{Count: cell.Count},
		})
	}
	return HeatmapResponse{Type: "FeatureCollection", Features: features}
}

type GetRegionsResponse struct {
	Regions []models.Region `json:"regions"`
}

type GetCitiesResponse struct {
	Cities []models.City `json:"cities"`
}

type GetDistrictsResponse struct {
	Districts []models.District `json:"districts"`
}
