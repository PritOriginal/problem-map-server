//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
)

// Timestamps are stored as TIMESTAMPTZ: an instant written with any UTC
// offset must be read back as the same instant, whatever the session time
// zone of the writer or the reader.

// TestTimestamptz_RoundTripWithOffset writes created_at with a +05:00
// offset and checks that the stored instant (rendered in UTC by the server)
// and the value read through the repository both match.
func (s *PostgresSuite) TestTimestamptz_RoundTripWithOffset() {
	yekaterinburg := time.FixedZone("UTC+5", 5*60*60)
	created := time.Date(2026, time.March, 10, 14, 30, 15, 0, yekaterinburg) // 09:30:15Z

	var id int
	s.Require().NoError(s.db.GetContext(s.ctx, &id, `
		INSERT INTO checks (user_id, mark_id, mark_status_id, mark_status_history_id, comment, result, created_at)
		VALUES ($1, $2, $3, $4, 'offset', true, $5)
		RETURNING check_id
	`, fxUserAlice, fxMarkNear, models.UnconfirmedStatus, fxHistoryMark1Initial, created))

	var storedUTC string
	s.Require().NoError(s.db.GetContext(s.ctx, &storedUTC,
		`SELECT to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS') FROM checks WHERE check_id = $1`, id))
	s.Equal("2026-03-10 09:30:15", storedUTC)

	got, err := s.checks.GetCheckById(s.ctx, id)
	s.Require().NoError(err)
	s.True(got.CreatedAt.Equal(created), "read %s, want the instant %s", got.CreatedAt, created)
}

// TestTimestamptz_NonUTCSession repeats the reads through a connection whose
// session time zone is not UTC: comparisons against bounds given in another
// zone and the instants read back must not shift.
func (s *PostgresSuite) TestTimestamptz_NonUTCSession() {
	db, err := sqlx.Connect("postgres", s.dsn+"&timezone=Asia/Yekaterinburg")
	s.Require().NoError(err)
	defer func() { _ = db.Close() }()

	var tz string
	s.Require().NoError(db.GetContext(s.ctx, &tz, "SHOW timezone"))
	s.Require().Equal("Asia/Yekaterinburg", tz)

	checks := postgres.NewChecks(db, trmsqlx.DefaultCtxGetter)
	marks := postgres.NewMarks(db, trmsqlx.DefaultCtxGetter)

	// The seeded checks of Bob are 1h and 2h old; the window bound is given
	// in yet another zone.
	now := time.Now().In(time.FixedZone("UTC-7", -7*60*60))
	n, err := checks.CountChecksByUserIdSince(s.ctx, fxUserBob, now.Add(-90*time.Minute))
	s.Require().NoError(err)
	s.Equal(1, n)

	utcCheck, err := s.checks.GetCheckById(s.ctx, fxCheckBobMark2)
	s.Require().NoError(err)
	localCheck, err := checks.GetCheckById(s.ctx, fxCheckBobMark2)
	s.Require().NoError(err)
	s.True(utcCheck.CreatedAt.Equal(localCheck.CreatedAt), "utc %s vs local %s", utcCheck.CreatedAt, localCheck.CreatedAt)

	// The backdated marks.created_at (seedNow - 40 days, in UTC) is read
	// back as the same instant.
	mark, err := marks.GetMarkById(s.ctx, fxMarkNear)
	s.Require().NoError(err)
	s.True(mark.CreatedAt.Equal(s.daysAgo(40)), "created_at %s, want %s", mark.CreatedAt, s.daysAgo(40))
}
