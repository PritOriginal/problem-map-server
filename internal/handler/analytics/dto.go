package analyticsrest

import (
	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// GetKPIRequest is bound from the query string of GET /analytics/kpi.
type GetKPIRequest struct {
	BoundaryID int `form:"boundary_id" binding:"omitempty,min=1"`
	MarkTypeID int `form:"mark_type_id" binding:"omitempty,min=1"`
	// From / To (RFC3339 or YYYY-MM-DD) bound marks' creation.
	From string `form:"from"`
	To   string `form:"to"`
}

// Filters converts the request to domain filters.
func (r GetKPIRequest) Filters() (models.AnalyticsFilters, error) {
	dates, err := listquery.ParseDateRange(r.From, r.To)
	if err != nil {
		return models.AnalyticsFilters{}, err
	}
	return models.AnalyticsFilters{
		BoundaryID: r.BoundaryID,
		MarkTypeID: r.MarkTypeID,
		DateRange:  dates,
	}, nil
}

// GetTimeseriesRequest is bound from the query string of GET /analytics/timeseries.
type GetTimeseriesRequest struct {
	GetKPIRequest
	Step string `form:"step" binding:"omitempty,oneof=day week month"`
}

// Filters converts the request to domain filters.
func (r GetTimeseriesRequest) Filters() (models.TimeseriesFilters, error) {
	base, err := r.GetKPIRequest.Filters()
	if err != nil {
		return models.TimeseriesFilters{}, err
	}
	return models.TimeseriesFilters{
		AnalyticsFilters: base,
		Step:             models.TimeseriesStep(r.Step),
	}, nil
}

// GetTopTypesRequest is bound from the query string of GET /analytics/top-types.
type GetTopTypesRequest struct {
	BoundaryID int `form:"boundary_id" binding:"omitempty,min=1"`
	// From / To (RFC3339 or YYYY-MM-DD) bound marks' creation.
	From  string `form:"from"`
	To    string `form:"to"`
	Limit int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// Filters converts the request to domain filters.
func (r GetTopTypesRequest) Filters() (models.TopTypesFilters, error) {
	dates, err := listquery.ParseDateRange(r.From, r.To)
	if err != nil {
		return models.TopTypesFilters{}, err
	}
	return models.TopTypesFilters{
		BoundaryID: r.BoundaryID,
		DateRange:  dates,
		Limit:      r.Limit,
	}, nil
}

type GetTimeseriesResponse struct {
	Timeseries []models.TimeseriesPoint `json:"timeseries"`
}

type GetTopTypesResponse struct {
	Types []models.TopType `json:"types"`
}
