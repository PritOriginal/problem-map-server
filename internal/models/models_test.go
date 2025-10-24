package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
)

func TestMark_UnmarshalJSON(t *testing.T) {
	expectedMark := Mark{
		ID:           1,
		Name:         "Свалка",
		Geom:         NewPoint(geom.Coord{41.402893, 52.700111}),
		MarkStatusID: 1,
		TypeMarkID:   1,
		UserID:       1,
		DistrictID:   2,
	}

	var mark Mark
	data := []byte(`{"geom":{"type":"Point","coordinates":[41.402893,52.700111]},"mark_id":1,"name":"Свалка", "mark_status_id": 1, "type_mark_id":1,"user_id":1,"district_id":2,"number_votes":0,"number_checks":0}`)
	err := json.Unmarshal(data, &mark)
	require.NoError(t, err)

	mark.Geom.Ewkb.SetSRID(4326)

	require.Equal(t, expectedMark, mark)
}

func TestMark_MarshalJSON(t *testing.T) {
	expectedMarkJSON := []byte(`{"mark_id":1,"name":"Свалка","geom":{"type":"Point","coordinates":[41.402893,52.700111]},"type_mark_id":1,"mark_status_id":1,"user_id":1,"district_id":2,"number_votes":0,"number_checks":0}`)

	mark := Mark{
		ID:           1,
		Name:         "Свалка",
		Geom:         NewPoint(geom.Coord{41.402893, 52.700111}),
		MarkStatusID: 1,
		TypeMarkID:   1,
		UserID:       1,
		DistrictID:   2,
	}

	markJSON, err := json.Marshal(mark)
	require.NoError(t, err)
	require.Equal(t, expectedMarkJSON, markJSON)
}
