package marksrest

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
)

// Exporter streams every mark matching the filters (see usecase.Export).
type Exporter interface {
	ExportMarks(ctx context.Context, filters models.GetMarksFilters, fn func(models.Mark) error) error
}

// Export formats and their media types.
const (
	ExportFormatGeoJSON = "geojson"
	ExportFormatCSV     = "csv"

	ContentTypeGeoJSON = "application/geo+json"
	ContentTypeCSV     = "text/csv; charset=utf-8"
)

// exportFlushEvery is the number of rows between flushes to the client.
const exportFlushEvery = 500

// exportWriteWindow is how long one chunk of the export may take to reach
// the client: the write deadline is pushed by this much on every flush, so
// a large export is not cut by the server's global write timeout while a
// stalled client still is.
const exportWriteWindow = 30 * time.Second

// ExportMarksRequest is bound from the query string of GET /marks/export:
// the GET /marks filters plus the format. Pagination is accepted but ignored.
type ExportMarksRequest struct {
	GetMarksRequest
	Format string `form:"format" binding:"required,oneof=geojson csv"`
}

// ExportMarks streams the markers matching the filters as a file
//
//	@Summary		Export markers
//	@Description	stream every marker matching the same filters as GET /marks (no pagination) as GeoJSON FeatureCollection or CSV (UTF-8 with BOM; a description starting with =, +, -, @ is prefixed with an apostrophe against formula injection). At most `export.max-rows` (50 000 by default) rows: a wider selection is rejected with 400 "narrow the filters". Rate limited per IP (2 per minute by default)
//	@Tags			marks
//	@Security		ApiKeyAuth
//	@Security		BearerAuth
//	@Produce		application/geo+json
//	@Produce		text/csv
//	@Param			format			query		string					true	"file format"	Enums(geojson, csv)
//	@Param			mark_type_ids	query		string					false	"filter by mark types, comma-separated ids"
//	@Param			mark_status_ids	query		string					false	"filter by mark statuses, comma-separated ids"
//	@Param			user_id			query		int						false	"filter by author"
//	@Param			bbox			query		string					false	"bounding box minLon,minLat,maxLon,maxLat (WGS84)"
//	@Param			created_from	query		string					false	"created_at >= (RFC3339)"
//	@Param			created_to		query		string					false	"created_at <= (RFC3339)"
//	@Param			sort			query		string					false	"sort column"	Enums(created_at, updated_at)	default(created_at)
//	@Param			order			query		string					false	"sort order"	Enums(asc, desc)				default(desc)
//	@Success		200				{string}	string					"the file; Content-Disposition: attachment"
//	@Failure		400				{object}	responses.Response[any]	"invalid filters or too many rows (narrow the filters)"
//	@Failure		429				{object}	responses.Response[any]
//	@Failure		500				{object}	responses.Response[any]
//	@Router			/marks/export [get]
func (h *handler) ExportMarks() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.ExportMarks"

		var req ExportMarksRequest
		if !listquery.Bind(c, h.log, &req) {
			return
		}
		filters, err := req.Filters()
		if err != nil {
			h.log.Debug("failed parse filters", logger.Err(err))
			responses.BadRequest(c, err.Error())
			return
		}

		enc := newExportEncoder(req.Format, c.Writer)
		started := false
		err = h.exporter.ExportMarks(c.Request.Context(), filters, func(m models.Mark) error {
			if !started {
				started = true
				if err := enc.Begin(); err != nil {
					return err
				}
			}
			return enc.Write(m)
		})
		if err != nil {
			if !started {
				if errorsIsTooLarge(err) {
					h.log.Debug(op, logger.Err(err))
					responses.BadRequest(c, "too many rows to export, narrow the filters")
					return
				}
				responses.FromError(c, h.log, op, err)
				return
			}
			// The status and headers are already sent: the body is cut
			// short and the client sees an incomplete file.
			h.log.Error(op, logger.Err(err))
			c.Abort()
			return
		}

		if !started {
			if err := enc.Begin(); err != nil {
				responses.FromError(c, h.log, op, err)
				return
			}
		}
		if err := enc.End(); err != nil {
			h.log.Error(op, logger.Err(err))
		}
	}
}

// exportEncoder writes one export file: Begin sends the headers and the
// preamble, Write one mark, End the trailer.
type exportEncoder interface {
	Begin() error
	Write(m models.Mark) error
	End() error
}

func newExportEncoder(format string, w gin.ResponseWriter) exportEncoder {
	name := "marks-" + time.Now().UTC().Format("20060102T150405Z")
	switch format {
	case ExportFormatCSV:
		return &csvExporter{w: w, filename: name + ".csv"}
	default:
		return &geoJSONExporter{w: w, filename: name + ".geojson"}
	}
}

func writeExportHeaders(w gin.ResponseWriter, contentType, filename string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	extendWriteDeadline(w)
	w.WriteHeader(http.StatusOK)
}

// extendWriteDeadline moves the response write deadline exportWriteWindow
// into the future. Writers that do not support deadlines (tests) are left
// alone.
func extendWriteDeadline(w gin.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(exportWriteWindow))
}

// flush sends the buffered part of the export to the client and extends
// the write deadline for the next chunk.
func flush(w gin.ResponseWriter) {
	w.Flush()
	extendWriteDeadline(w)
}

// flushEvery flushes w to the client every exportFlushEvery rows so a large
// export streams instead of buffering in the server.
func flushEvery(w gin.ResponseWriter, rows int) {
	if rows%exportFlushEvery == 0 {
		flush(w)
	}
}

// csvCell neutralises spreadsheet formula injection: a cell starting with
// a formula trigger (=, +, -, @) or a control character is prefixed with an
// apostrophe so Excel/LibreOffice show it as text instead of evaluating it.
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + s
	}
	return s
}

// GeoJSONFeature is one mark of the GeoJSON export.
type GeoJSONFeature struct {
	Type       string         `json:"type"`
	ID         int            `json:"id"`
	Geometry   *models.Point  `json:"geometry"`
	Properties MarkProperties `json:"properties"`
}

// MarkProperties are the non-geometry fields of a mark in the export.
type MarkProperties struct {
	MarkID         int                   `json:"mark_id"`
	Description    string                `json:"description"`
	MarkTypeID     int                   `json:"mark_type_id"`
	MarkStatusID   models.MarkStatusType `json:"mark_status_id"`
	UserID         int                   `json:"user_id"`
	FollowersCount int                   `json:"followers_count"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// NewGeoJSONFeature converts a mark to a GeoJSON feature.
func NewGeoJSONFeature(m models.Mark) GeoJSONFeature {
	return GeoJSONFeature{
		Type:     "Feature",
		ID:       m.ID,
		Geometry: m.Geom,
		Properties: MarkProperties{
			MarkID:         m.ID,
			Description:    m.Description,
			MarkTypeID:     m.MarkTypeID,
			MarkStatusID:   m.MarkStatusID,
			UserID:         m.UserID,
			FollowersCount: m.FollowersCount,
			CreatedAt:      m.CreatedAt,
			UpdatedAt:      m.UpdatedAt,
		},
	}
}

type geoJSONExporter struct {
	w        gin.ResponseWriter
	filename string
	enc      *json.Encoder
	rows     int
}

func (e *geoJSONExporter) Begin() error {
	writeExportHeaders(e.w, ContentTypeGeoJSON, e.filename)
	e.enc = json.NewEncoder(e.w)
	_, err := io.WriteString(e.w, `{"type":"FeatureCollection","features":[`)
	return err
}

func (e *geoJSONExporter) Write(m models.Mark) error {
	if e.rows > 0 {
		if _, err := io.WriteString(e.w, ","); err != nil {
			return err
		}
	}
	// Encode appends a newline after every feature, which keeps the file
	// valid JSON and readable line by line.
	if err := e.enc.Encode(NewGeoJSONFeature(m)); err != nil {
		return err
	}
	e.rows++
	flushEvery(e.w, e.rows)
	return nil
}

func (e *geoJSONExporter) End() error {
	if _, err := io.WriteString(e.w, "]}\n"); err != nil {
		return err
	}
	flush(e.w)
	return nil
}

// CSVHeader is the first row of the CSV export.
var CSVHeader = []string{"mark_id", "longitude", "latitude", "description", "mark_type_id", "mark_status_id", "user_id", "followers_count", "created_at", "updated_at"}

// utf8BOM lets Excel detect UTF-8.
const utf8BOM = "\xEF\xBB\xBF"

type csvExporter struct {
	w        gin.ResponseWriter
	filename string
	csv      *csv.Writer
	rows     int
}

func (e *csvExporter) Begin() error {
	writeExportHeaders(e.w, ContentTypeCSV, e.filename)
	if _, err := io.WriteString(e.w, utf8BOM); err != nil {
		return err
	}
	e.csv = csv.NewWriter(e.w)
	e.csv.UseCRLF = true
	return e.csv.Write(CSVHeader)
}

func (e *csvExporter) Write(m models.Mark) error {
	lon, lat := "", ""
	if m.Geom != nil && m.Geom.Valid() {
		lon = strconv.FormatFloat(m.Geom.Ewkb.X(), 'f', -1, 64)
		lat = strconv.FormatFloat(m.Geom.Ewkb.Y(), 'f', -1, 64)
	}
	if err := e.csv.Write([]string{
		strconv.Itoa(m.ID),
		lon,
		lat,
		csvCell(m.Description),
		strconv.Itoa(m.MarkTypeID),
		strconv.Itoa(int(m.MarkStatusID)),
		strconv.Itoa(m.UserID),
		strconv.Itoa(m.FollowersCount),
		m.CreatedAt.UTC().Format(time.RFC3339),
		m.UpdatedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return err
	}
	e.rows++
	if e.rows%exportFlushEvery == 0 {
		e.csv.Flush()
		if err := e.csv.Error(); err != nil {
			return err
		}
		flush(e.w)
	}
	return nil
}

func (e *csvExporter) End() error {
	e.csv.Flush()
	if err := e.csv.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	flush(e.w)
	return nil
}

// errorsIsTooLarge reports whether err is usecase.ErrExportTooLarge.
func errorsIsTooLarge(err error) bool {
	return errors.Is(err, usecase.ErrExportTooLarge)
}
