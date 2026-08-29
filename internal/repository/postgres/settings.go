package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/guregu/null/v6"
	"github.com/jmoiron/sqlx"
)

// SettingsRepository stores the admin-editable runtime settings (table
// settings) and reads their change log (settings_history, filled by a
// database trigger on every write that changes the value).
type SettingsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewSettings(db *sqlx.DB, c *trmsqlx.CtxGetter) *SettingsRepository {
	return &SettingsRepository{db: db, getter: c}
}

// GetSetting returns the stored document under key (repository.ErrNotFound
// when the key was never written).
func (r *SettingsRepository) GetSetting(ctx context.Context, key string) (models.Setting, error) {
	const op = "storage.postgres.GetSetting"

	var s models.Setting
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &s, `SELECT key, value, updated_by, updated_at FROM settings WHERE key = $1`, key); err != nil {
		return s, wrapPgError(op, err)
	}
	return s, nil
}

// SetSetting writes value under key (insert or update) and records who did
// it; updatedBy is stored as NULL when invalid. Writing the stored value
// again is a no-op (no updated_at/updated_by change, no history row).
func (r *SettingsRepository) SetSetting(ctx context.Context, key string, value json.RawMessage, updatedBy null.Int) error {
	const op = "storage.postgres.SetSetting"

	query := `
		INSERT INTO settings (key, value, updated_by, updated_at) VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
		WHERE settings.value IS DISTINCT FROM EXCLUDED.value`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, query, key, []byte(value), updatedBy); err != nil {
		return wrapPgError(op, err)
	}
	return nil
}

// GetSettingsHistory lists the latest changes of key, newest first.
func (r *SettingsRepository) GetSettingsHistory(ctx context.Context, key string, limit int) ([]models.SettingChange, error) {
	const op = "storage.postgres.GetSettingsHistory"

	changes := []models.SettingChange{}
	query := `SELECT id, key, old, new, updated_by, updated_at FROM settings_history
		WHERE key = $1 ORDER BY updated_at DESC, id DESC LIMIT $2`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &changes, query, key, limit); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return changes, nil
}
