package maprest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

const (
	// HeatmapCacheTTL is how long a heatmap response is served from cache;
	// the cache key includes the full query string.
	HeatmapCacheTTL = 60 * time.Second
	// DictionaryCacheTTL is how long the slowly changing reference data
	// (boundaries, regions, cities, districts) stays cached.
	DictionaryCacheTTL = 24 * time.Hour
	// GeoJSONMaxAge is the Cache-Control max-age of the boundary GeoJSON:
	// boundaries change only with an OSM re-import.
	GeoJSONMaxAge = 24 * time.Hour
)

type Map interface {
	GetAdminBoundaries(ctx context.Context, filters models.GetAdminBoundaryFilters) ([]models.AdminBoundary, error)
	GetAdminBoundariesMarksCount(ctx context.Context, filters models.GetAdminBoundaryMarksCountFilters) ([]models.AdminBoundaryMarksCount, error)
	GetAdminBoundaryById(ctx context.Context, id int) (models.AdminBoundary, error)
	GetHeatmap(ctx context.Context, filters models.HeatmapFilters) ([]models.HeatmapCell, error)
	GetRegions(ctx context.Context) ([]models.Region, error)
	GetCities(ctx context.Context) ([]models.City, error)
	GetDistricts(ctx context.Context) ([]models.District, error)
}

type handler struct {
	log *slog.Logger
	uc  Map
}

func Register(r *gin.Engine, log *slog.Logger, uc Map, cacher mwcache.Cacher) {
	handler := &handler{log: log, uc: uc}

	mapRoute := r.Group("/map")
	{
		mapRoute.GET("admin-boundaries/marks/count", handler.GetAdminBoundariesMarksCount())
		// The bare GeoJSON document is cached with its own content type and
		// a long max-age (see GeoJSONMaxAge).
		geojson := mapRoute.Group("")
		{
			geojson.Use(mwcache.New(cacher, DictionaryCacheTTL, mwcache.WithMaxAge(GeoJSONMaxAge)))
			geojson.GET("admin-boundaries/:file", handler.GetAdminBoundaryGeoJSON())
		}
		heatmap := mapRoute.Group("")
		{
			heatmap.Use(mwcache.New(cacher, HeatmapCacheTTL))
			heatmap.GET("heatmap", handler.GetHeatmap())
		}
		cache := mapRoute.Group("")
		{
			cache.Use(mwcache.New(cacher, DictionaryCacheTTL))
			cache.GET("admin-boundaries", handler.GetAdminBoundaries())
			cache.GET("regions", handler.GetRegions())
			cache.GET("cities", handler.GetCities())
			cache.GET("districts", handler.GetDistricts())
		}
	}
}

// GetAdminBoundaries lists all existing administrative boundaries
//
//	@Summary		List administrative boundaries
//	@Description	admin boundaries
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Param			admin_levels	query		[]number	false	"filter by admin level"
//	@Param			If-None-Match	header		string		false	"ETag of a previous response; 304 when the data did not change"
//	@Success		200				{object}	responses.Response[maprest.GetAdminBoundariesResponse]
//	@Header			200				{string}	ETag		"validator for If-None-Match"
//	@Header			200				{string}	Cache-Control	"public, max-age=60"
//	@Success		304				"not modified"
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries [get]
func (h *handler) GetAdminBoundaries() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetAdminBoundaries"

		adminLevels, err := handlers.QueryIntArray(c, "admin_levels")
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		boundaries, err := h.uc.GetAdminBoundaries(c.Request.Context(), models.GetAdminBoundaryFilters{
			AdminLevels: adminLevels,
		})
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetAdminBoundariesResponse{
			AdminBoundaries: boundaries,
		})
	}
}

// GetAdminBoundaryGeoJSON returns one administrative boundary as a GeoJSON Feature
//
//	@Summary		Administrative boundary as GeoJSON
//	@Description	one boundary with its MultiPolygon geometry as a GeoJSON Feature (`application/geo+json`): `properties` carry `name` and `admin_level`
//	@Tags			map
//	@Produce		application/geo+json
//	@Param			id				path		int		true	"boundary id (the path is /map/admin-boundaries/{id}.geojson)"
//	@Param			If-None-Match	header		string	false	"ETag of a previous response; 304 when the boundary did not change"
//	@Success		200				{object}	maprest.AdminBoundaryFeature
//	@Header			200				{string}	ETag			"validator for If-None-Match"
//	@Header			200				{string}	Cache-Control	"public, max-age=86400"
//	@Success		304				"not modified"
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		404				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries/{id}.geojson [get]
func (h *handler) GetAdminBoundaryGeoJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetAdminBoundaryGeoJSON"

		id, ok := strings.CutSuffix(c.Param("file"), GeoJSONSuffix)
		if !ok {
			responses.NotFound(c, responses.MsgNotFound)
			return
		}
		boundaryID, err := strconv.Atoi(id)
		if err != nil || boundaryID <= 0 {
			responses.BadRequest(c, "invalid boundary id")
			return
		}

		boundary, err := h.uc.GetAdminBoundaryById(c.Request.Context(), boundaryID)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		// Cache-Control and ETag are added by the cache middleware.
		body, err := json.Marshal(NewAdminBoundaryFeature(boundary))
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}
		c.Data(http.StatusOK, ContentTypeGeoJSON, body)
	}
}

// GetAdminBoundariesMarksCount display the count of markers of all administrative boundaries
//
//	@Summary		The count of markers of all administrative boundaries
//	@Description	the count of markers of all administrative boundaries
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Param			admin_levels	query		[]number	false	"filter by admin level"
//	@Param			mark_type_ids	query		[]number	false	"filter by mark type"
//	@Param			mark_status_ids	query		[]number	false	"filter by mark status"
//	@Param			from			query		string		false	"marks created at or after (RFC3339)"
//	@Param			to				query		string		false	"marks created at or before (RFC3339)"
//	@Success		200				{object}	responses.Response[maprest.GetAdminBoundariesMarksCountResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/admin-boundaries/marks/count [get]
func (h *handler) GetAdminBoundariesMarksCount() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetAdminBoundariesMarksCount"

		var req GetAdminBoundariesMarksCountRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			responses.BadRequest(c, err.Error())
			return
		}

		boundariesCount, err := h.uc.GetAdminBoundariesMarksCount(c.Request.Context(), filters)
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetAdminBoundariesMarksCountResponse{
			AdminBoundaries: boundariesCount,
		})
	}
}

// GetHeatmap bins the marks inside a bbox into a hexagonal grid
//
//	@Summary		Marks heatmap
//	@Description	GeoJSON FeatureCollection of hexagons (EPSG:3857 grid, returned in WGS84) with the number of marks in each; empty cells are omitted. At most 5000 cells: a finer grid is rejected with 400, increase cell_m. Cached for 60 seconds per query.
//	@Tags			map
//	@Produce		json
//	@Param			bbox			query		string		true	"minLon,minLat,maxLon,maxLat"
//	@Param			cell_m			query		number		false	"hexagon size (center-to-vertex) in ground meters (10..100000)"	default(250)
//	@Param			mark_type_ids	query		[]number	false	"filter by mark type"
//	@Param			mark_status_ids	query		[]number	false	"filter by mark status"
//	@Success		200				{object}	responses.Response[maprest.HeatmapResponse]
//	@Failure		400				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/map/heatmap [get]
func (h *handler) GetHeatmap() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetHeatmap"

		var req GetHeatmapRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			responses.BadRequest(c, err.Error())
			return
		}

		cells, err := h.uc.GetHeatmap(c.Request.Context(), filters)
		if err != nil {
			if errors.Is(err, usecase.ErrTooManyHeatmapCells) {
				h.log.Debug(op, slog.String("err", err.Error()))
				responses.BadRequest(c, MsgTooManyCells)
				return
			}
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, NewHeatmapResponse(cells))
	}
}

// GetCities lists all existing regions
//
//	@Summary		List regions
//	@Description	get regions
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[maprest.GetRegionsResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/map/regions [get]
func (h *handler) GetRegions() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetRegions"

		regions, err := h.uc.GetRegions(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetRegionsResponse{
			Regions: regions,
		})
	}
}

// GetCities lists all existing cities
//
//	@Summary		List cities
//	@Description	get cities
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[maprest.GetCitiesResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/map/cities [get]
func (h *handler) GetCities() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetCities"

		cities, err := h.uc.GetCities(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetCitiesResponse{
			Cities: cities,
		})
	}
}

// GetDistricts lists all existing districts
//
//	@Summary		List districts
//	@Description	get districts
//	@Tags			map
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	responses.Response[maprest.GetDistrictsResponse]
//	@Failure		500	{object}	responses.Response[any]
//	@Router			/map/districts [get]
func (h *handler) GetDistricts() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "maprest.GetDistricts"

		districts, err := h.uc.GetDistricts(c.Request.Context())
		if err != nil {
			responses.FromError(c, h.log, op, err)
			return
		}

		responses.OK(c, GetDistrictsResponse{
			Districts: districts,
		})
	}
}
