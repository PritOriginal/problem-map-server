// Package analyticsrest exposes aggregated statistics over marks: KPI
// summary, time series and the mark-type ranking.
package analyticsrest

import (
	"context"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

type Analytics interface {
	GetKPI(ctx context.Context, filters models.AnalyticsFilters) (models.KPI, error)
	GetTimeseries(ctx context.Context, filters models.TimeseriesFilters) ([]models.TimeseriesPoint, error)
	GetTopTypes(ctx context.Context, filters models.TopTypesFilters) ([]models.TopType, error)
}

type handler struct {
	log *slog.Logger
	uc  Analytics
}

func Register(r *gin.Engine, log *slog.Logger, uc Analytics) {
	handler := &handler{log: log, uc: uc}

	analytics := r.Group("/analytics")
	{
		analytics.GET("kpi", handler.GetKPI())
		analytics.GET("timeseries", handler.GetTimeseries())
		analytics.GET("top-types", handler.GetTopTypes())
	}
}

// GetKPI summarizes the marks matching the filters
//
//	@Summary		KPI summary
//	@Description	Totals, per-status counts, confirmation/closing durations (hours, from the status history), refuted share and stale open marks. All filters are optional.
//	@Tags			analytics
//	@Produce		json
//	@Param			boundary_id		query		int		false	"only marks inside this admin boundary"
//	@Param			mark_type_id	query		int		false	"only marks of this type"
//	@Param			from			query		string	false	"marks created at or after (RFC3339)"
//	@Param			to				query		string	false	"marks created at or before (RFC3339)"
//	@Success		200				{object}	responses.Response[models.KPI]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/analytics/kpi [get]
func (h *handler) GetKPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "analyticsrest.GetKPI"

		var req GetKPIRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			responses.BadRequest(c, err.Error())
			return
		}

		kpi, err := h.uc.GetKPI(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, kpi)
	}
}

// GetTimeseries returns per-period counts of created marks and status transitions
//
//	@Summary		Marks time series
//	@Description	Number of marks created and transitions to confirmed / closed / refuted per period; empty periods are returned with zeros. Defaults: step=day, to=now, from=to minus 30 days (12 weeks / 12 months for coarser steps).
//	@Tags			analytics
//	@Produce		json
//	@Param			boundary_id		query		int		false	"only marks inside this admin boundary"
//	@Param			mark_type_id	query		int		false	"only marks of this type"
//	@Param			from			query		string	false	"start of the range (RFC3339)"
//	@Param			to				query		string	false	"end of the range (RFC3339)"
//	@Param			step			query		string	false	"bucket size"	Enums(day, week, month)	default(day)
//	@Success		200				{object}	responses.Response[analyticsrest.GetTimeseriesResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/analytics/timeseries [get]
func (h *handler) GetTimeseries() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "analyticsrest.GetTimeseries"

		var req GetTimeseriesRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			responses.BadRequest(c, err.Error())
			return
		}

		points, err := h.uc.GetTimeseries(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetTimeseriesResponse{Timeseries: points})
	}
}

// GetTopTypes ranks mark types by the number of marks
//
//	@Summary		Top mark types
//	@Description	Mark types ordered by the number of matching marks with their share of the total.
//	@Tags			analytics
//	@Produce		json
//	@Param			boundary_id	query		int		false	"only marks inside this admin boundary"
//	@Param			from		query		string	false	"marks created at or after (RFC3339)"
//	@Param			to			query		string	false	"marks created at or before (RFC3339)"
//	@Param			limit		query		int		false	"number of rows (1..100)"	default(10)
//	@Success		200			{object}	responses.Response[analyticsrest.GetTopTypesResponse]
//	@Failure		400			{object}	responses.Response[any]
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/analytics/top-types [get]
func (h *handler) GetTopTypes() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "analyticsrest.GetTopTypes"

		var req GetTopTypesRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			responses.BadRequest(c, err.Error())
			return
		}

		types, err := h.uc.GetTopTypes(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetTopTypesResponse{Types: types})
	}
}
