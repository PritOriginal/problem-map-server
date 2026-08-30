package marksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"

	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/brianvoe/gofakeit/v7"
	mock "github.com/stretchr/testify/mock"
)

// batchFiles says how many photos to attach to each item index; a negative
// count sends that many broken (non-image) files instead.
type batchFiles map[int]int

// batchRequest builds the multipart request of POST /marks/batch. The
// suite registers the handler without idempotency keys, so these tests also
// cover the nil *idempotency.Keys wiring.
func (suite *MarksSuite) batchRequest(items string, files batchFiles) *http.Request {
	b := &bytes.Buffer{}
	mpw := multipart.NewWriter(b)
	suite.Require().NoError(mpw.WriteField(marksrest.BatchItemsField, items))

	for i, n := range files {
		broken := n < 0
		if broken {
			n = -n
		}
		for j := range n {
			fw, err := mpw.CreateFormFile(marksrest.BatchPhotosField(i), fmt.Sprintf("p%d-%d.jpg", i, j))
			suite.Require().NoError(err)
			data := gofakeit.ImageJpeg(10, 10)
			if broken {
				data = []byte("not an image")
			}
			_, err = io.Copy(fw, bytes.NewReader(data))
			suite.Require().NoError(err)
		}
	}
	suite.Require().NoError(mpw.Close())

	req := httptest.NewRequest(http.MethodPost, "/marks/batch", b)
	req.Header.Set("Authorization", suite.bearer(models.RoleUser))
	req.Header.Set("Content-Type", mpw.FormDataContentType())
	return req
}

// batchItemsJSON renders the "items" field from the given operations.
func (suite *MarksSuite) batchItemsJSON(items []marksrest.BatchAddMarkItem) string {
	raw, err := json.Marshal(items)
	suite.Require().NoError(err)
	return string(raw)
}

func (suite *MarksSuite) doBatch(req *http.Request) (*httptest.ResponseRecorder, responses.Response[marksrest.BatchAddMarksResponse]) {
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)

	var body responses.Response[marksrest.BatchAddMarksResponse]
	if w.Code == http.StatusOK {
		suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	}
	return w, body
}

func item(lon, lat float64, photos int) marksrest.BatchAddMarkItem {
	return marksrest.BatchAddMarkItem{Longitude: lon, Latitude: lat, MarkTypeID: 3, Photos: photos}
}

// TestAddMarksBatchMixed: a mixed batch answers 200 with one result per
// item, in the request order, and a rejected item cancels nothing.
func (suite *MarksSuite) TestAddMarksBatchMixed() {
	similar := []models.MarkWithDistance{{Mark: models.Mark{ID: 5, MarkTypeID: 3}, DistanceM: 12.5}}

	items := []marksrest.BatchAddMarkItem{
		item(41.1, 52.7, 1),
		item(41.2, 52.7, 2),
		item(41.3, 52.7, 1),
	}

	suite.uc.On("AddMarks", mock.Anything, mock.MatchedBy(func(prepared []models.NewMark) bool {
		// The usecase must receive the items in the request order, with
		// the files of every index attached to it.
		return len(prepared) == 3 &&
			prepared[0].Mark.Geom.Ewkb.X() == 41.1 && len(prepared[0].Photos) == 1 &&
			prepared[1].Mark.Geom.Ewkb.X() == 41.2 && len(prepared[1].Photos) == 2 &&
			prepared[2].Mark.Geom.Ewkb.X() == 41.3 && len(prepared[2].Photos) == 1
	})).Once().Return([]models.BatchAddResult{
		{Status: models.BatchStatusCreated, MarkID: 42},
		{Status: models.BatchStatusDuplicate, SimilarMarks: similar, Err: &usecase.SimilarMarksError{Marks: similar}},
		{Status: models.BatchStatusFailed, Err: fmt.Errorf("op: %w", errors.New("boom"))},
	})

	w, body := suite.doBatch(suite.batchRequest(suite.batchItemsJSON(items), batchFiles{0: 1, 1: 2, 2: 1}))

	suite.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	suite.True(body.Success)
	suite.Require().Len(body.Payload.Results, 3)

	suite.Equal(0, body.Payload.Results[0].Index)
	suite.Equal(models.BatchStatusCreated, body.Payload.Results[0].Status)
	suite.Equal(42, body.Payload.Results[0].MarkId)

	suite.Equal(1, body.Payload.Results[1].Index)
	suite.Equal(models.BatchStatusDuplicate, body.Payload.Results[1].Status)
	suite.Require().NotNil(body.Payload.Results[1].Error)
	suite.Equal(responses.CodeSimilarMarks, body.Payload.Results[1].Error.Code)
	suite.Require().Len(body.Payload.Results[1].SimilarMarks, 1)
	suite.Equal(5, body.Payload.Results[1].SimilarMarks[0].ID)

	suite.Equal(2, body.Payload.Results[2].Index)
	suite.Equal(models.BatchStatusFailed, body.Payload.Results[2].Status)
	suite.Require().NotNil(body.Payload.Results[2].Error)
	suite.Equal(marksrest.CodeInternal, body.Payload.Results[2].Error.Code)
}

// TestAddMarksBatchBrokenItems: an item the handler itself rejects becomes
// a failed result and never reaches the usecase, while its neighbours are
// applied as usual.
func (suite *MarksSuite) TestAddMarksBatchBrokenItems() {
	longDescription := strings.Repeat("a", models.MaxMarkDescriptionLen+1)

	tests := []struct {
		name    string
		item    marksrest.BatchAddMarkItem
		files   int
		wantMsg string
	}{
		{name: "InvalidPhoto", item: item(41.2, 52.7, 1), files: -1, wantMsg: "invalid photo"},
		{name: "PhotosCountMismatch", item: item(41.2, 52.7, 2), files: 1, wantMsg: "photos count mismatch"},
		{name: "NoPhotos", item: item(41.2, 52.7, 0), files: 0, wantMsg: "at least one photo"},
		{name: "TooManyPhotos", item: item(41.2, 52.7, handlers.MaxPhotos+1), files: handlers.MaxPhotos + 1, wantMsg: "too many files"},
		{name: "NoMarkType", item: marksrest.BatchAddMarkItem{Longitude: 41.2, Latitude: 52.7, Photos: 1}, files: 1, wantMsg: "mark_type_id"},
		{
			name:    "LongDescription",
			item:    marksrest.BatchAddMarkItem{Longitude: 41.2, Latitude: 52.7, MarkTypeID: 3, Photos: 1, Description: longDescription},
			files:   1,
			wantMsg: "description",
		},
		{
			name:    "InvalidCoordinates",
			item:    item(200, 52.7, 1),
			files:   1,
			wantMsg: "longitude",
		},
		{
			name:    "InvalidKey",
			item:    marksrest.BatchAddMarkItem{Key: "not-a-uuid", Longitude: 41.2, Latitude: 52.7, MarkTypeID: 3, Photos: 1},
			files:   1,
			wantMsg: "UUID",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()

			items := []marksrest.BatchAddMarkItem{item(41.1, 52.7, 1), tt.item, item(41.3, 52.7, 1)}
			// Only the good neighbours reach the usecase, still in order.
			suite.uc.On("AddMarks", mock.Anything, mock.MatchedBy(func(prepared []models.NewMark) bool {
				return len(prepared) == 2 &&
					prepared[0].Mark.Geom.Ewkb.X() == 41.1 &&
					prepared[1].Mark.Geom.Ewkb.X() == 41.3
			})).Once().Return([]models.BatchAddResult{
				{Status: models.BatchStatusCreated, MarkID: 1},
				{Status: models.BatchStatusCreated, MarkID: 3},
			})

			w, body := suite.doBatch(suite.batchRequest(suite.batchItemsJSON(items), batchFiles{0: 1, 1: tt.files, 2: 1}))

			suite.Require().Equal(http.StatusOK, w.Code, w.Body.String())
			suite.Require().Len(body.Payload.Results, 3)
			suite.Equal(models.BatchStatusCreated, body.Payload.Results[0].Status)
			suite.Equal(1, body.Payload.Results[0].MarkId)
			suite.Equal(models.BatchStatusFailed, body.Payload.Results[1].Status)
			suite.Require().NotNil(body.Payload.Results[1].Error)
			suite.Equal(marksrest.CodeInvalidArgument, body.Payload.Results[1].Error.Code)
			suite.Contains(body.Payload.Results[1].Error.Message, tt.wantMsg)
			suite.Equal(models.BatchStatusCreated, body.Payload.Results[2].Status)
			suite.Equal(3, body.Payload.Results[2].MarkId)
		})
	}
}

// TestAddMarksBatchKeyEchoed: an item's key is echoed back so the client
// can retire it from its offline queue.
func (suite *MarksSuite) TestAddMarksBatchKeyEchoed() {
	key := "3f1e6b52-2f1a-4a0e-9c6e-8f6c1f4d1a11"
	it := item(41.1, 52.7, 1)
	it.Key = key

	suite.uc.On("AddMarks", mock.Anything, mock.Anything).Once().
		Return([]models.BatchAddResult{{Status: models.BatchStatusCreated, MarkID: 7}})

	w, body := suite.doBatch(suite.batchRequest(suite.batchItemsJSON([]marksrest.BatchAddMarkItem{it}), batchFiles{0: 1}))

	suite.Require().Equal(http.StatusOK, w.Code, w.Body.String())
	suite.Require().Len(body.Payload.Results, 1)
	suite.Equal(key, body.Payload.Results[0].Key)
	suite.Equal(models.BatchStatusCreated, body.Payload.Results[0].Status)
	suite.Equal(7, body.Payload.Results[0].MarkId)
}

// TestAddMarksBatchRejected: a malformed batch is rejected as a whole.
func (suite *MarksSuite) TestAddMarksBatchRejected() {
	tooMany := make([]marksrest.BatchAddMarkItem, handlers.MaxBatchItems+1)
	for i := range tooMany {
		tooMany[i] = item(41.1, 52.7, 1)
	}

	tests := []struct {
		name     string
		items    string
		omit     bool
		wantCode string
	}{
		{name: "NotJSON", items: "{", wantCode: marksrest.CodeItemsInvalid},
		{name: "NotAnArray", items: `{"key":"x"}`, wantCode: marksrest.CodeItemsInvalid},
		{name: "Empty", items: `[]`, wantCode: marksrest.CodeItemsInvalid},
		{name: "Missing", omit: true, wantCode: marksrest.CodeItemsInvalid},
		{name: "TooMany", items: suite.batchItemsJSON(tooMany), wantCode: marksrest.CodeItemsTooMany},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()

			req := suite.batchRequest(tt.items, batchFiles{0: 1})
			if tt.omit {
				// Rebuild the body without the items field at all.
				b := &bytes.Buffer{}
				mpw := multipart.NewWriter(b)
				suite.Require().NoError(mpw.WriteField("other", "x"))
				suite.Require().NoError(mpw.Close())
				req = httptest.NewRequest(http.MethodPost, "/marks/batch", b)
				req.Header.Set("Authorization", suite.bearer(models.RoleUser))
				req.Header.Set("Content-Type", mpw.FormDataContentType())
			}

			w := httptest.NewRecorder()
			suite.r.ServeHTTP(w, req)

			suite.Require().Equal(http.StatusBadRequest, w.Code, w.Body.String())
			var body responses.Response[any]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body))
			suite.False(body.Success)
			suite.Require().NotNil(body.Error)
			suite.Equal(tt.wantCode, body.Error.Code)
		})
	}
}

// TestAddMarksBatchRequiresAuth: the route sits behind the JWT middleware.
func (suite *MarksSuite) TestAddMarksBatchRequiresAuth() {
	req := suite.batchRequest(suite.batchItemsJSON([]marksrest.BatchAddMarkItem{item(41.1, 52.7, 1)}), batchFiles{0: 1})
	req.Header.Del("Authorization")

	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)

	suite.Equal(http.StatusUnauthorized, w.Code, w.Body.String())
}
