//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/repository/postgres"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twpayne/go-geom"
)

const postgisImage = "postgis/postgis:16-3.4"

// Fixture identifiers. TRUNCATE ... RESTART IDENTITY in SetupTest makes them
// deterministic for every test.
const (
	fxUserAlice = 1 // home near markNear
	fxUserBob   = 2 // home far away from every mark

	fxMarkNear     = 1 // inside admin boundary "Центр", type 1, status unconfirmed
	fxMarkInside   = 2 // inside admin boundary "Центр", type 2, status confirmed
	fxMarkFar      = 3 // outside every boundary, type 1, status under review
	fxBoundaryMain = 1 // "Центр" — contains marks 1 and 2
	fxBoundaryVoid = 2 // "Пустой" — contains no marks
)

// Coordinates (lon, lat) used by the fixtures.
var (
	coordAliceHome = geom.Coord{41.4006, 52.6999}
	coordBobHome   = geom.Coord{37.6173, 55.7558} // Moscow, ~700 km away
	coordMarkNear  = geom.Coord{41.4028, 52.7001} // ~150 m from Alice
	coordMarkIn    = geom.Coord{41.4100, 52.7050}
	coordMarkFar   = geom.Coord{41.6000, 52.9000}
)

// PostgresSuite runs the repositories against a real PostGIS container with
// the project migrations applied.
type PostgresSuite struct {
	suite.Suite

	ctx context.Context
	db  *sqlx.DB
	dsn string
	trm *manager.Manager

	users         *postgres.UsersRepository
	marks         *postgres.MarksRepository
	checks        *postgres.ChecksRepository
	tasks         *postgres.TasksRepository
	maps          *postgres.MapRepository
	analytics     *postgres.AnalyticsRepository
	notifications *postgres.NotificationsRepository

	// seedNow anchors the backdated timestamps of the fixtures (UTC, whole
	// seconds) so tests can compute expected periods and durations exactly.
	seedNow time.Time
}

func TestPostgresSuite(t *testing.T) {
	suite.Run(t, new(PostgresSuite))
}

func (s *PostgresSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := tcpostgres.Run(s.ctx, postgisImage,
		tcpostgres.WithDatabase("problem_map_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	s.Require().NoError(err, "start postgis container")
	// Register termination right away so a failure later in SetupSuite still
	// removes the container.
	testcontainers.CleanupContainer(s.T(), container)

	dsn, err := container.ConnectionString(s.ctx, "sslmode=disable")
	s.Require().NoError(err)
	s.dsn = dsn

	s.migrateUp(dsn)

	db, err := sqlx.Connect("postgres", dsn)
	s.Require().NoError(err)
	s.db = db

	s.trm = manager.Must(trmsqlx.NewDefaultFactory(db))
	getter := trmsqlx.DefaultCtxGetter

	s.users = postgres.NewUsers(db, getter)
	s.marks = postgres.NewMarks(db, getter)
	s.checks = postgres.NewChecks(db, getter)
	s.tasks = postgres.NewTasks(db, getter)
	s.maps = postgres.NewMap(db, getter)
	s.notifications = postgres.NewNotifications(db, getter)
	s.analytics = postgres.NewAnalytics(db, getter)
}

func (s *PostgresSuite) TearDownSuite() {
	if s.db != nil {
		_ = s.db.Close()
	}
}

// SetupTest wipes all mutable tables and seeds the fixtures again, so every
// test starts from the same deterministic state.
func (s *PostgresSuite) SetupTest() {
	s.truncate()
	s.seed()
}

func (s *PostgresSuite) migrateUp(dsn string) {
	_, file, _, ok := runtime.Caller(0)
	s.Require().True(ok)
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")

	m, err := migrate.New("file://"+migrationsDir, dsn)
	s.Require().NoError(err, "create migrate")
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		s.Require().NoError(err, "apply migrations")
	}
}

func (s *PostgresSuite) truncate() {
	_, err := s.db.ExecContext(s.ctx, `
		TRUNCATE TABLE
			notifications, user_devices,
			rating_events, checks, tasks, mark_status_history, mark_followers, marks, users, admin_boundaries, types_marks,
			districts, cities, regions
		RESTART IDENTITY CASCADE
	`)
	s.Require().NoError(err, "truncate")

	// Mark types are re-seeded with fresh ids, so their translations from the
	// migration would point at the wrong rows.
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM translations WHERE entity = 'mark_type'`)
	s.Require().NoError(err, "truncate mark type translations")
}

func (s *PostgresSuite) seed() {
	// Migrations only add types 'Освещение' and 'Информационные и визуальные
	// дефекты' on top of the base ones from insert.sql, so the full list is
	// seeded here with deterministic ids.
	// Type 4 deliberately has no English translation to cover the fallback.
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO types_marks (name, code) VALUES
			('Мусор', 'garbage'), ('Зелёные зоны и парки', 'green_zones'), ('Освещение', 'lighting'),
			('Информационные и визуальные дефекты', 'visual_defects');
		INSERT INTO translations (entity, entity_id, lang, name)
		SELECT 'mark_type', type_mark_id, 'ru', name FROM types_marks;
		INSERT INTO translations (entity, entity_id, lang, name) VALUES
			('mark_type', 1, 'en', 'Garbage'), ('mark_type', 2, 'en', 'Green zones and parks'),
			('mark_type', 3, 'en', 'Lighting');
	`)
	s.Require().NoError(err, "seed mark types")

	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO users (name, login, password_hash, home_point, rating, role) VALUES
			('Alice', 'alice', 'hash-alice', ST_SetSRID(ST_MakePoint($1, $2), 4326), 10, 'user'),
			('Bob',   'bob',   'hash-bob',   ST_SetSRID(ST_MakePoint($3, $4), 4326), 0,  'moderator');
	`, coordAliceHome.X(), coordAliceHome.Y(), coordBobHome.X(), coordBobHome.Y())
	s.Require().NoError(err, "seed users")

	// The insert trigger writes the initial history row for every mark.
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO marks (description, geom, type_mark_id, user_id) VALUES
			('Свалка у дома',   ST_SetSRID(ST_MakePoint($1, $2), 4326), 1, 1),
			('Разбитая лавка',  ST_SetSRID(ST_MakePoint($3, $4), 4326), 2, 1),
			('Далёкая яма',     ST_SetSRID(ST_MakePoint($5, $6), 4326), 1, 2);
	`, coordMarkNear.X(), coordMarkNear.Y(), coordMarkIn.X(), coordMarkIn.Y(), coordMarkFar.X(), coordMarkFar.Y())
	s.Require().NoError(err, "seed marks")

	// Status transitions go through UPDATE so the trigger builds the history
	// chain. History ids after seeding:
	//   1: mark1 NULL->1
	//   2: mark2 NULL->1
	//   3: mark3 NULL->1
	//   4: mark2 1->2 (prev 2)
	//   5: mark3 1->3 (prev 3)
	_, err = s.db.ExecContext(s.ctx, `
		UPDATE marks SET mark_status_id = 2 WHERE mark_id = 2;
		UPDATE marks SET mark_status_id = 3 WHERE mark_id = 3;

		INSERT INTO checks (user_id, mark_id, mark_status_id, mark_status_history_id, comment, result, created_at) VALUES
			(2, 1, 1, 1, 'Bob confirms mark 1',  true,  NOW() - INTERVAL '2 hour'),
			(1, 2, 1, 2, 'Alice on mark 2 v1',   true,  NOW() - INTERVAL '3 hour'),
			(2, 2, 2, 4, 'Bob on mark 2 v2',     false, NOW() - INTERVAL '1 hour');

		INSERT INTO tasks (name, user_id, mark_id, status_id) VALUES
			('Проверить свалку', 1, 1, 1),
			('Проверить лавку',  1, 2, 2),
			('Проверить яму',    2, 3, 1);

		-- Authors follow their own marks; nobody else does.
		INSERT INTO mark_followers (user_id, mark_id) VALUES (1, 1), (1, 2), (2, 3);

		INSERT INTO admin_boundaries (osm_id, name, admin_level, geom) VALUES
			(1001, 'Центр', 8, ST_SetSRID(ST_Multi(ST_MakeEnvelope(41.39, 52.69, 41.42, 52.71)), 4326)),
			(1002, 'Пустой', 8, ST_SetSRID(ST_Multi(ST_MakeEnvelope(41.50, 52.80, 41.52, 52.82)), 4326));
	`)
	s.Require().NoError(err, "seed statuses, checks, tasks and admin boundaries")

	// Backdate creation and status transitions so the analytics have
	// deterministic durations and periods (relative to seedNow):
	//   mark1: created 40 days ago, still unconfirmed (stale open mark)
	//   mark2: created 10 days ago, confirmed 48 h later
	//   mark3: created 5 days ago, under review 24 h later
	s.seedNow = time.Now().UTC().Truncate(time.Second)
	// lib/pq forbids several statements in one parametrized query, hence
	// one UPDATE per table.
	_, err = s.db.ExecContext(s.ctx, `
		UPDATE marks SET
			created_at = $1::timestamptz - CASE mark_id
				WHEN 1 THEN INTERVAL '40 days' WHEN 2 THEN INTERVAL '10 days' ELSE INTERVAL '5 days' END,
			updated_at = $1::timestamptz - CASE mark_id
				WHEN 1 THEN INTERVAL '40 days' WHEN 2 THEN INTERVAL '8 days' ELSE INTERVAL '4 days' END
	`, s.seedNow)
	s.Require().NoError(err, "backdate marks")

	_, err = s.db.ExecContext(s.ctx, `
		UPDATE mark_status_history SET changed_at = $1::timestamptz - CASE id
			WHEN 1 THEN INTERVAL '40 days'
			WHEN 2 THEN INTERVAL '10 days'
			WHEN 3 THEN INTERVAL '5 days'
			WHEN 4 THEN INTERVAL '8 days'
			ELSE INTERVAL '4 days' END
	`, s.seedNow)
	s.Require().NoError(err, "backdate history")
}

// daysAgo returns seedNow minus the given number of days.
func (s *PostgresSuite) daysAgo(days int) time.Time {
	return s.seedNow.AddDate(0, 0, -days)
}

// ids collects the identifiers of items in order.
func ids[T any](items []T, id func(T) int) []int {
	out := make([]int, 0, len(items))
	for _, it := range items {
		out = append(out, id(it))
	}
	return out
}

// countRows is a helper for asserting side effects directly in the database.
func (s *PostgresSuite) countRows(table, where string, args ...any) int {
	var n int
	s.Require().NoError(s.db.GetContext(s.ctx, &n, "SELECT COUNT(*) FROM "+table+" WHERE "+where, args...))
	return n
}
