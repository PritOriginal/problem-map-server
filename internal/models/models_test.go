package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
)

func TestMark_UnmarshalJSON(t *testing.T) {
	expectedMark := Mark{
		ID:           1,
		Description:  "Свалка",
		Geom:         NewPoint(geom.Coord{41.402893, 52.700111}),
		MarkStatusID: 1,
		MarkTypeID:   1,
		UserID:       1,
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	var mark Mark
	data := []byte(`{"geom":{"type":"Point","coordinates":[41.402893,52.700111]},"mark_id":1,"description":"Свалка","mark_status_id":1,"mark_type_id":1,"user_id":1,"number_votes":0,"number_checks":0,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`)
	err := json.Unmarshal(data, &mark)
	require.NoError(t, err)

	mark.Geom.Ewkb.SetSRID(4326)

	require.Equal(t, expectedMark, mark)
}

func TestMark_MarshalJSON(t *testing.T) {
	expectedMarkJSON := []byte(`{"mark_id":1,"description":"Свалка","geom":{"type":"Point","coordinates":[41.402893,52.700111]},"mark_type_id":1,"mark_status_id":1,"user_id":1,"number_votes":0,"number_checks":0,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`)

	mark := Mark{
		ID:           1,
		Description:  "Свалка",
		Geom:         NewPoint(geom.Coord{41.402893, 52.700111}),
		MarkStatusID: 1,
		MarkTypeID:   1,
		UserID:       1,
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	markJSON, err := json.Marshal(mark)
	require.NoError(t, err)
	require.Equal(t, expectedMarkJSON, markJSON)
}
