package marksrest

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"unicode/utf8"

	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// BatchPath is the sub-path of the batch creation endpoint.
const BatchPath = "batch"

const (
	// BatchItemsField is the multipart field carrying the JSON array of
	// operations, in the order they must be applied.
	BatchItemsField = "items"
	// BatchPhotosPrefix is the prefix of the per-item file fields; see
	// BatchPhotosField.
	BatchPhotosPrefix = "photos"
)

// Machine-readable codes of a per-item error (BatchMarkResult.Error.Code).
// They mirror the status codes the single-mark endpoint answers with, so
// that a client can treat an item exactly as it treats POST /marks.
const (
	CodeInvalidArgument  = "invalid_argument"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeForbidden        = "forbidden"
	CodeUnauthorized     = "unauthorized"
	CodeTooManyRequests  = "too_many_requests"
	CodeInternal         = "internal"
	CodeItemsInvalid     = "invalid_items"
	CodeItemsTooMany     = "too_many_items"
	MsgBatchItemsInvalid = "invalid items: expected a JSON array of operations"
)

// AddMarksBatch creates several marks in one request
//
//	@Summary		Add marks in a batch
//	@Description	Apply a queue of offline mark creations in one multipart request. `items` is a JSON array of operations **in the order they must be applied**, and the server applies them one after another, so duplicate detection works exactly as it does for a sequence of `POST /marks`: every item is checked against the marks the earlier items of the same batch have just created.
//	@Description
//	@Description	The photos of the item at index `i` are sent in the file fields `photos.i` (several files per index are allowed, in order); `items[i].photos` must match how many files are sent for that index.
//	@Description
//	@Description	The answer is always `200` when the batch itself is well-formed: every item carries its own result (`created`, `replayed`, `duplicate`, `failed`), and a rejected item never cancels the others. An item may set `"force": true` to skip its own duplicate detection, and `"key"` (a UUID) to make itself idempotent: repeating a batch replays the items whose key already created a mark instead of creating it twice.
//	@Tags			marks
//	@Accept			mpfd
//	@Produce		json
//	@Security		BearerAuth
//	@Param			items		formData	string												true	"JSON array of operations, at most 20, in the order they must be applied. Item: {key?: uuid, longitude, latitude, mark_type_id, description?, force?, photos}"	example([{\"longitude\":41.44,\"latitude\":52.72,\"mark_type_id\":3,\"photos\":1}])
//	@Param			photos.0	formData	file												true	"Photos of the item at index 0 (repeat the field for several files); use photos.1, photos.2, … for the next items, at most 5 files per item"
//	@Success		200			{object}	responses.Response[marksrest.BatchAddMarksResponse]	"per-item results in the request order; check every item's status"
//	@Failure		400			{object}	responses.Response[any]								"the batch itself is malformed: `items` missing or not a JSON array (`error.code` is `invalid_items`), or more than 20 items (`too_many_items`)"
//	@Failure		401			{object}	responses.Response[any]
//	@Failure		413			{object}	responses.Response[any]	"the request body is larger than the batch limit"
//	@Failure		500			{object}	responses.Response[any]
//	@Router			/marks/batch [post]
func (h *handler) AddMarksBatch() gin.HandlerFunc {
	return func(c *gin.Context) {
		const op = "marksrest.AddMarksBatch"

		userId, err := middleware.UserIDFromClaims(c)
		if err != nil {
			h.log.Debug("invalid token", logger.Err(err))
			responses.Unauthorized(c, "invalid token")
			return
		}

		form, err := c.MultipartForm()
		if err != nil {
			h.log.Debug("failed parsing multipart form", logger.Err(err))
			responses.BadRequest(c, "invalid multipart form")
			return
		}

		items, ok := parseBatchItems(c, h.log, form)
		if !ok {
			return
		}

		ctx := viewerContext(c, userId)

		// Every item is validated on its own: a broken item becomes a
		// failed result and the rest of the batch is still applied.
		results := make([]BatchMarkResult, len(items))
		prepared := make([]models.NewMark, 0, len(items))
		// positions[k] is the index in results of prepared[k].
		positions := make([]int, 0, len(items))
		for i, item := range items {
			results[i] = BatchMarkResult{Index: i, Key: item.Key}

			newMark, err := item.newMark(userId, form.File[BatchPhotosField(i)])
			if err != nil {
				h.log.Debug(op, slog.Int("index", i), logger.Err(err))
				results[i].fail(CodeInvalidArgument, err.Error())
				continue
			}

			if item.Key != "" {
				// A key that could not be reserved (no Redis, or a batch
				// still holding it) is simply not idempotent: the item is
				// then applied as if it carried no key at all.
				markID, replayed, _ := h.keys.Reserve(ctx, userId, item.Key)
				if replayed {
					results[i].Status = models.BatchStatusReplayed
					results[i].MarkId = int(markID)
					continue
				}
			}

			prepared = append(prepared, newMark)
			positions = append(positions, i)
		}

		for k, res := range h.uc.AddMarks(ctx, prepared) {
			i := positions[k]
			results[i].apply(res, h.log, op)

			key := items[i].Key
			switch {
			case res.Status == models.BatchStatusCreated:
				h.keys.Commit(ctx, userId, key, res.MarkID)
			case key != "":
				// Nothing was created under the key, so free it: the
				// client may retry that item with the same key.
				h.keys.Release(ctx, userId, key)
			}
		}

		h.log.Info("add marks batch",
			slog.Int("user_id", userId),
			slog.Int("items", len(items)),
			slog.Int("applied", len(prepared)),
		)
		responses.OK(c, BatchAddMarksResponse{Results: results})
	}
}

// parseBatchItems decodes the "items" field. A malformed batch is answered
// with 400 (the whole request is rejected) and ok is false; broken single
// items are not its business.
func parseBatchItems(c *gin.Context, log *slog.Logger, form *multipart.Form) ([]BatchAddMarkItem, bool) {
	raw := form.Value[BatchItemsField]
	if len(raw) == 0 {
		responses.FailWithCode(c, http.StatusBadRequest, CodeItemsInvalid, MsgBatchItemsInvalid)
		return nil, false
	}

	var items []BatchAddMarkItem
	if err := json.Unmarshal([]byte(raw[0]), &items); err != nil {
		log.Debug("failed decoding items", logger.Err(err))
		responses.FailWithCode(c, http.StatusBadRequest, CodeItemsInvalid, MsgBatchItemsInvalid)
		return nil, false
	}
	if len(items) == 0 {
		responses.FailWithCode(c, http.StatusBadRequest, CodeItemsInvalid, "items must not be empty")
		return nil, false
	}
	if len(items) > handlers.MaxBatchItems {
		responses.FailWithCode(c, http.StatusBadRequest, CodeItemsTooMany,
			fmt.Sprintf("too many items: %d > %d", len(items), handlers.MaxBatchItems))
		return nil, false
	}
	return items, true
}

// newMark validates the item together with the files sent for it and
// builds the domain input. Returned errors are safe to show to the client.
func (item BatchAddMarkItem) newMark(userId int, fheaders []*multipart.FileHeader) (models.NewMark, error) {
	if item.Key != "" && (len(item.Key) > 64 || uuid.Validate(item.Key) != nil) {
		return models.NewMark{}, errors.New("invalid key: expected a UUID")
	}
	if item.MarkTypeID <= 0 {
		return models.NewMark{}, errors.New("mark_type_id is required")
	}
	if utf8.RuneCountInString(item.Description) > models.MaxMarkDescriptionLen {
		return models.NewMark{}, fmt.Errorf("description is longer than %d characters", models.MaxMarkDescriptionLen)
	}
	if len(fheaders) == 0 {
		return models.NewMark{}, errors.New("at least one photo is required")
	}
	if item.Photos != len(fheaders) {
		return models.NewMark{}, fmt.Errorf("photos count mismatch: declared %d, sent %d", item.Photos, len(fheaders))
	}

	point, err := models.NewPointLonLat(item.Longitude, item.Latitude)
	if err != nil {
		return models.NewMark{}, err
	}

	photos, err := handlers.ParsePhotos(fheaders)
	if err != nil {
		return models.NewMark{}, err
	}

	return models.NewMark{
		Mark: models.Mark{
			Geom:        point,
			MarkTypeID:  item.MarkTypeID,
			UserID:      userId,
			Description: item.Description,
		},
		Photos: photos,
		Force:  item.Force,
	}, nil
}

// apply copies the usecase outcome into the response item, mapping the
// error of a rejected item the way FromError maps it to a status code.
func (r *BatchMarkResult) apply(res models.BatchAddResult, log *slog.Logger, op string) {
	r.Status = res.Status
	switch res.Status {
	case models.BatchStatusCreated:
		r.MarkId = int(res.MarkID)
	case models.BatchStatusDuplicate:
		r.SimilarMarks = res.SimilarMarks
		r.Error = &responses.ErrorInfo{Code: responses.CodeSimilarMarks, Message: "similar marks nearby"}
	default:
		code, msg := batchErrorInfo(res.Err)
		if code == CodeInternal {
			log.Error(op, slog.Int("index", r.Index), logger.Err(res.Err))
		} else {
			log.Debug(op, slog.Int("index", r.Index), logger.Err(res.Err))
		}
		r.Status = models.BatchStatusFailed
		r.Error = &responses.ErrorInfo{Code: code, Message: msg}
	}
}

// fail marks the item as rejected before it ever reached the usecase.
func (r *BatchMarkResult) fail(code, message string) {
	r.Status = models.BatchStatusFailed
	r.Error = &responses.ErrorInfo{Code: code, Message: message}
}

// batchErrorInfo maps an item failure to the code and message shown to the
// client; internal failures are reported without details, as elsewhere.
func batchErrorInfo(err error) (code, message string) {
	kind := usecase.Kind(err)
	if errors.Is(err, handlers.ErrInvalidPhoto) || errors.Is(err, handlers.ErrBadRequest) {
		kind = usecase.KindInvalidArgument
	}

	switch kind {
	case usecase.KindNotFound:
		return CodeNotFound, responses.MsgNotFound
	case usecase.KindConflict:
		return CodeConflict, responses.MsgConflict
	case usecase.KindUnauthorized:
		return CodeUnauthorized, responses.MsgUnauthorized
	case usecase.KindForbidden:
		return CodeForbidden, responses.MsgForbidden
	case usecase.KindTooManyRequests:
		return CodeTooManyRequests, responses.MsgTooManyReq
	case usecase.KindInvalidArgument:
		return CodeInvalidArgument, responses.MsgBadRequest
	default:
		return CodeInternal, responses.MsgInternal
	}
}
