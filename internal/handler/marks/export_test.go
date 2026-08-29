package marksrest_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
)

type ExportSuite struct {
	suite.Suite
	r        *gin.Engine
	exporter *marksrest.MockExporter
	limited  int
}

func (suite *ExportSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte("1234")})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.exporter = marksrest.NewMockExporter(suite.T())
	suite.limited = 0

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	marksrest.Register(suite.r, slogdiscard.NewDiscardLogger(), marksrest.Params{
		AuthMiddleware: authMiddleware,
		Usecase:        marksrest.NewMockMarks(suite.T()),
		StatusUpdater:  marksrest.NewMockStatusUpdater(suite.T()),
		Exporter:       suite.exporter,
		ExportRateLimit: func(c *gin.Context) {
			suite.limited++
			c.Next()
		},
	})
}

func TestExport(t *testing.T) {
	suite.Run(t, new(ExportSuite))
}

func exportMarks() []models.Mark {
	at := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	return []models.Mark{
		{ID: 1, Description: "Свалка, \"у дома\"", Geom: models.NewPoint(geom.Coord{41.44, 52.72}), MarkTypeID: 1, MarkStatusID: models.ConfirmedStatus, UserID: 7, FollowersCount: 2, CreatedAt: at, UpdatedAt: at.Add(time.Hour)},
		{ID: 2, Description: "multi\nline", Geom: models.NewPoint(geom.Coord{41.5, 52.8}), MarkTypeID: 2, MarkStatusID: models.UnconfirmedStatus, UserID: 8, CreatedAt: at, UpdatedAt: at},
	}
}

// streaming feeds marks to fn, cancelling nothing: it mimics the use case.
func streaming(marks []models.Mark) func(context.Context, models.GetMarksFilters, func(models.Mark) error) error {
	return func(_ context.Context, _ models.GetMarksFilters, fn func(models.Mark) error) error {
		for _, m := range marks {
			if err := fn(m); err != nil {
				return err
			}
		}
		return nil
	}
}

func (suite *ExportSuite) do(query string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/marks/export"+query, nil)
	suite.r.ServeHTTP(w, req)
	return w
}

func (suite *ExportSuite) TestExportGeoJSON() {
	suite.exporter.On("ExportMarks", mock.Anything, mock.MatchedBy(func(f models.GetMarksFilters) bool {
		return f.BBox != nil && f.BBox.MinLon == 41 && f.MarkTypeIds[0] == 1 && f.UserID == 7 && f.Sort == models.MarksSortUpdatedAt
	}), mock.Anything).Once().Return(streaming(exportMarks()))

	w := suite.do("?format=geojson&bbox=41,52,42,53&mark_type_ids=1&user_id=7&sort=updated_at&order=asc&limit=5")

	suite.Equal(http.StatusOK, w.Code, w.Body.String())
	suite.Equal(marksrest.ContentTypeGeoJSON, w.Header().Get("Content-Type"))
	suite.Regexp(`^attachment; filename="marks-\d{8}T\d{6}Z\.geojson"$`, w.Header().Get("Content-Disposition"))
	suite.Equal(1, suite.limited, "the rate limiter wraps the route")

	var fc struct {
		Type     string                     `json:"type"`
		Features []marksrest.GeoJSONFeature `json:"features"`
	}
	suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &fc), w.Body.String())
	suite.Equal("FeatureCollection", fc.Type)
	suite.Require().Len(fc.Features, 2)
	suite.Equal("Feature", fc.Features[0].Type)
	suite.Equal(1, fc.Features[0].ID)
	suite.Equal(1, fc.Features[0].Properties.MarkID)
	suite.Equal(models.ConfirmedStatus, fc.Features[0].Properties.MarkStatusID)
	suite.Equal(2, fc.Features[0].Properties.FollowersCount)
	suite.InDelta(41.44, fc.Features[0].Geometry.Ewkb.X(), 1e-9)
	suite.InDelta(52.72, fc.Features[0].Geometry.Ewkb.Y(), 1e-9)
	// Every feature sits on its own line (json.Encoder), the file stays valid JSON.
	suite.Equal(3, strings.Count(w.Body.String(), "\n"))
}

func (suite *ExportSuite) TestExportGeoJSONEmpty() {
	suite.exporter.On("ExportMarks", mock.Anything, mock.Anything, mock.Anything).Once().Return(streaming(nil))

	w := suite.do("?format=geojson")

	suite.Equal(http.StatusOK, w.Code)
	suite.Equal(marksrest.ContentTypeGeoJSON, w.Header().Get("Content-Type"))
	suite.JSONEq(`{"type":"FeatureCollection","features":[]}`, w.Body.String())
}

func (suite *ExportSuite) TestExportCSV() {
	suite.exporter.On("ExportMarks", mock.Anything, mock.Anything, mock.Anything).Once().Return(streaming(exportMarks()))

	w := suite.do("?format=csv")

	suite.Equal(http.StatusOK, w.Code, w.Body.String())
	suite.Equal(marksrest.ContentTypeCSV, w.Header().Get("Content-Type"))
	suite.Regexp(`^attachment; filename="marks-\d{8}T\d{6}Z\.csv"$`, w.Header().Get("Content-Disposition"))

	body := w.Body.String()
	suite.True(strings.HasPrefix(body, "\xEF\xBB\xBF"), "UTF-8 BOM for Excel")
	suite.Contains(body, "\r\n", "CRLF line endings")

	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\xEF\xBB\xBF"))).ReadAll()
	suite.Require().NoError(err)
	suite.Require().Len(records, 3)
	suite.Equal(marksrest.CSVHeader, records[0])
	suite.Equal([]string{"1", "41.44", "52.72", "Свалка, \"у дома\"", "1", "2", "7", "2", "2026-08-29T10:00:00Z", "2026-08-29T11:00:00Z"}, records[1])
	suite.Equal("multi\nline", records[2][3], "newlines are quoted, not split")
}

func (suite *ExportSuite) TestExportErrors() {
	tests := []struct {
		name       string
		query      string
		err        error
		statusCode int
		message    string
	}{
		{name: "MissingFormat", query: "", statusCode: http.StatusBadRequest},
		{name: "UnknownFormat", query: "?format=xlsx", statusCode: http.StatusBadRequest},
		{name: "BadBBox", query: "?format=csv&bbox=1,2,3", statusCode: http.StatusBadRequest},
		{name: "BadDate", query: "?format=csv&created_from=yesterday", statusCode: http.StatusBadRequest},
		{name: "TooLarge", query: "?format=csv", err: fmt.Errorf("op: %w (70000 rows)", usecase.ErrExportTooLarge), statusCode: http.StatusBadRequest, message: "narrow the filters"},
		{name: "InvalidArgument", query: "?format=geojson", err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Internal", query: "?format=geojson", err: errors.New("db down"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.err != nil {
				suite.exporter.On("ExportMarks", mock.Anything, mock.Anything, mock.Anything).Once().Return(tt.err)
			}

			w := suite.do(tt.query)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.message != "" {
				suite.Contains(w.Body.String(), tt.message)
			}
		})
	}
}

func (suite *ExportSuite) TestExportMidStreamFailureCutsBody() {
	suite.exporter.On("ExportMarks", mock.Anything, mock.Anything, mock.Anything).Once().
		Return(func(_ context.Context, _ models.GetMarksFilters, fn func(models.Mark) error) error {
			if err := fn(exportMarks()[0]); err != nil {
				return err
			}
			return errors.New("connection lost")
		})

	w := suite.do("?format=geojson")

	// Headers were already sent with 200; the collection is not closed.
	suite.Equal(http.StatusOK, w.Code)
	suite.Equal(marksrest.ContentTypeGeoJSON, w.Header().Get("Content-Type"))
	suite.False(strings.HasSuffix(strings.TrimSpace(w.Body.String()), "]}"))
	suite.Error(json.Unmarshal(w.Body.Bytes(), &struct{}{}))
}

func (suite *ExportSuite) TestExportRouteAbsentWithoutExporter() {
	r := gin.New()
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte("1234")})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())
	marksrest.Register(r, slogdiscard.NewDiscardLogger(), marksrest.Params{
		AuthMiddleware: authMiddleware,
		Usecase:        marksrest.NewMockMarks(suite.T()),
		StatusUpdater:  marksrest.NewMockStatusUpdater(suite.T()),
	})

	// Without the route "export" falls through to /marks/:id, which rejects
	// the non-numeric id.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/marks/export?format=csv", nil))
	suite.Equal(http.StatusBadRequest, w.Code)
}
