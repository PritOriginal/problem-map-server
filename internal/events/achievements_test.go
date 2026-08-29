package events_test

import (
	"encoding/json"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNewBadgeEarnedIsDeterministic(t *testing.T) {
	a := events.NewBadgeEarned(7, "first_mark")
	b := events.NewBadgeEarned(7, "first_mark")
	other := events.NewBadgeEarned(8, "first_mark")
	otherBadge := events.NewBadgeEarned(7, "reporter_10")

	require.Equal(t, a.EventID, b.EventID)
	require.NotEqual(t, a.EventID, other.EventID)
	require.NotEqual(t, a.EventID, otherBadge.EventID)
	_, err := uuid.Parse(a.EventID)
	require.NoError(t, err)
	require.Equal(t, events.SchemaVersion, a.Version)
	require.Equal(t, events.SubjectBadgeEarned, a.Subject())

	payload, err := json.Marshal(a)
	require.NoError(t, err)
	require.JSONEq(t, `{"v":1,"event_id":"`+a.EventID+`","user_id":7,"badge_code":"first_mark"}`, string(payload))
}

func TestNewTaskCompleted(t *testing.T) {
	ev := events.NewTaskCompleted(9, 2, 5, 77)
	require.NotEmpty(t, ev.EventID)
	require.Equal(t, events.SubjectTaskCompleted, ev.Subject())
	require.Equal(t, events.SchemaVersion, ev.Version)

	payload, err := json.Marshal(ev)
	require.NoError(t, err)
	require.JSONEq(t, `{"v":1,"event_id":"`+ev.EventID+`","task_id":9,"user_id":2,"mark_id":5,"check_id":77}`, string(payload))
}
