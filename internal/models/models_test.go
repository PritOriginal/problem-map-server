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
	data := []byte(`{"geom":{"type":"Point","coordinates":[41.402893,52.700111]},"mark_id":1,"description":"Свалка","mark_status_id":1,"mark_type_id":1,"user_id":1,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","followers_count":0,"is_following":false}`)
	err := json.Unmarshal(data, &mark)
	require.NoError(t, err)

	mark.Geom.Ewkb.SetSRID(4326)

	require.Equal(t, expectedMark, mark)
}

func TestMark_MarshalJSON(t *testing.T) {
	expectedMarkJSON := []byte(`{"mark_id":1,"description":"Свалка","geom":{"type":"Point","coordinates":[41.402893,52.700111]},"mark_type_id":1,"mark_status_id":1,"user_id":1,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","followers_count":0,"is_following":false,"organization_id":null,"sla_due_at":null,"is_overdue":false,"comments_count":0,"hidden":false,"merged_into_id":null}`)

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

func TestUser_Public(t *testing.T) {
	u := User{Id: 1, Name: "n", Login: "l", PasswordHash: "h", HomePoint: NewPoint(geom.Coord{1, 2}), Rating: 3, Role: RoleAdmin}

	got := u.Public()

	require.Equal(t, User{Id: 1, Name: "n", Rating: 3, Role: RoleAdmin}, got)
	// The receiver is a copy: the original keeps its private fields.
	require.Equal(t, "l", u.Login)
	require.NotNil(t, u.HomePoint)
}
