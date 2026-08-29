//go:build integration

package postgres_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
)

// i18nMigrationVersion is the version of 000037_add_i18n.
const i18nMigrationVersion = 37

// TestMigration_I18nDuplicateNames re-runs 000037_add_i18n over dictionaries
// with duplicated and unknown names: the codes must stay unique (the
// duplicates get an `_<id>` suffix, unknown names a synthetic code) so the
// NOT NULL / UNIQUE constraints of the migration hold.
func (s *PostgresSuite) TestMigration_I18nDuplicateNames() {
	_, file, _, ok := runtime.Caller(0)
	s.Require().True(ok)
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")

	m, err := migrate.New("file://"+migrationsDir, s.dsn)
	s.Require().NoError(err)
	defer func() { _, _ = m.Close() }()

	// Migrate to an explicit version: later migrations (000038, ...) may
	// follow 000037, so a relative step would revert the wrong one.
	s.Require().NoError(m.Migrate(i18nMigrationVersion-1), "revert 000037")
	// Whatever happens, leave the schema fully migrated for the next tests.
	defer func() {
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			s.Require().NoError(err, "re-apply 000037")
		}
	}()

	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO types_marks (name) VALUES ('Мусор'), ('Мусор'), ('Ямы во дворе');
	`)
	s.Require().NoError(err)

	s.Require().NoError(m.Migrate(i18nMigrationVersion), "apply 000037 with duplicated names")

	var got []struct {
		ID   int    `db:"type_mark_id"`
		Name string `db:"name"`
		Code string `db:"code"`
	}
	s.Require().NoError(s.db.SelectContext(s.ctx, &got,
		`SELECT type_mark_id, name, code FROM types_marks WHERE type_mark_id > 4 ORDER BY type_mark_id`))
	s.Require().Len(got, 3)

	// The seeded 'Мусор' (id 1) keeps the bare code; the duplicates are suffixed.
	s.Equal("garbage_"+strconv.Itoa(got[0].ID), got[0].Code)
	s.Equal("garbage_"+strconv.Itoa(got[1].ID), got[1].Code)
	s.Equal("type_"+strconv.Itoa(got[2].ID), got[2].Code)

	var seeded string
	s.Require().NoError(s.db.GetContext(s.ctx, &seeded, `SELECT code FROM types_marks WHERE type_mark_id = 1`))
	s.Equal("garbage", seeded)

	// Only the row with a known code got an English translation; the
	// suffixed duplicates fall back to Russian at read time.
	var en int
	s.Require().NoError(s.db.GetContext(s.ctx, &en,
		`SELECT COUNT(*) FROM translations WHERE entity = 'mark_type' AND lang = 'en' AND entity_id > 4`))
	s.Equal(0, en)
}
