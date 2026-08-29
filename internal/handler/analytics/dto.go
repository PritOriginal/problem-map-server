package analyticsrest

import (
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

// parseDateRange parses optional RFC3339 bounds. Returned errors are safe to
// show to the client.
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

// GetKPIRequest is bound from the query string of GET /analytics/kpi.
type GetKPIRequest struct {
	BoundaryID int `form:"boundary_id" binding:"omitempty,min=1"`
	MarkTypeID int `form:"mark_type_id" binding:"omitempty,min=1"`
	// From / To are RFC3339 timestamps bounding marks' creation.
	From string `form:"from"`
	To   string `form:"to"`
}

// Filters converts the request to domain filters.
func (r GetKPIRequest) Filters() (models.AnalyticsFilters, error) {
	dates, err := parseDateRange(r.From, r.To)
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
	// From / To are RFC3339 timestamps bounding marks' creation.
	From  string `form:"from"`
	To    string `form:"to"`
	Limit int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// Filters converts the request to domain filters.
func (r GetTopTypesRequest) Filters() (models.TopTypesFilters, error) {
	dates, err := parseDateRange(r.From, r.To)
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
