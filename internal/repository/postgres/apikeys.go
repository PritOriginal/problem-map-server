package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// APIKeysRepository stores the API keys of the open-data endpoints.
type APIKeysRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewAPIKeys(db *sqlx.DB, c *trmsqlx.CtxGetter) *APIKeysRepository {
	return &APIKeysRepository{
		db:     db,
		getter: c,
	}
}

// apiKeyColumns is a column list, not a credential.
const apiKeyColumns = "api_key_id, owner_user_id, name, key_hash, prefix, scopes, rate_limit_per_min, active, last_used_at, expires_at, created_at" //nolint:gosec // column list

// apiKeyRow is the scan target: text[] needs pq.StringArray.
type apiKeyRow struct {
	models.APIKey
	Scopes pq.StringArray `db:"scopes"`
}

func (r apiKeyRow) model() models.APIKey {
	k := r.APIKey
	k.Scopes = []string(r.Scopes)
	return k
}

func apiKeyModels(rows []apiKeyRow) []models.APIKey {
	out := make([]models.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.model())
	}
	return out
}

// AddAPIKey inserts k and returns its id (repository.ErrExists on a hash
// collision, repository.ErrInvalidReference on an unknown owner).
func (r *APIKeysRepository) AddAPIKey(ctx context.Context, k models.APIKey) (int64, error) {
	const op = "storage.postgres.AddAPIKey"

	query := `
			INSERT INTO
				api_keys (owner_user_id, name, key_hash, prefix, scopes, rate_limit_per_min, active, expires_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING api_key_id
			`
	var id int64
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query,
		k.OwnerUserID, k.Name, k.KeyHash, k.Prefix, pq.Array(k.Scopes), k.RateLimitPerMin, k.Active, k.ExpiresAt); err != nil {
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

// GetAPIKeyById returns the key (repository.ErrNotFound when missing).
func (r *APIKeysRepository) GetAPIKeyById(ctx context.Context, id int) (models.APIKey, error) {
	const op = "storage.postgres.GetAPIKeyById"

	return r.get(ctx, op, "api_key_id = $1", id)
}

// GetAPIKeyByHash returns the key with the SHA-256 (repository.ErrNotFound
// when missing); revoked and expired keys are returned too, the caller
// decides.
func (r *APIKeysRepository) GetAPIKeyByHash(ctx context.Context, hash string) (models.APIKey, error) {
	const op = "storage.postgres.GetAPIKeyByHash"

	return r.get(ctx, op, "key_hash = $1", hash)
}

func (r *APIKeysRepository) get(ctx context.Context, op, where string, args ...any) (models.APIKey, error) {
	var row apiKeyRow
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &row, "SELECT "+apiKeyColumns+" FROM api_keys WHERE "+where, args...); err != nil {
		return models.APIKey{}, wrapPgError(op, err)
	}

	return row.model(), nil
}

// GetAPIKeysByOwner lists the user's keys, oldest first.
func (r *APIKeysRepository) GetAPIKeysByOwner(ctx context.Context, ownerUserID int) ([]models.APIKey, error) {
	const op = "storage.postgres.GetAPIKeysByOwner"

	return r.list(ctx, op, "WHERE owner_user_id = $1", ownerUserID)
}

// GetAllAPIKeys lists every key, oldest first (admin view).
func (r *APIKeysRepository) GetAllAPIKeys(ctx context.Context) ([]models.APIKey, error) {
	const op = "storage.postgres.GetAllAPIKeys"

	return r.list(ctx, op, "")
}

func (r *APIKeysRepository) list(ctx context.Context, op, where string, args ...any) ([]models.APIKey, error) {
	rows := []apiKeyRow{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	query := "SELECT " + apiKeyColumns + " FROM api_keys " + where + " ORDER BY api_key_id"
	if err := tr.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return apiKeyModels(rows), nil
}

// RevokeAPIKey deactivates the key (repository.ErrNotFound when missing;
// revoking twice is not an error).
func (r *APIKeysRepository) RevokeAPIKey(ctx context.Context, id int) error {
	const op = "storage.postgres.RevokeAPIKey"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "UPDATE api_keys SET active = FALSE WHERE api_key_id = $1", id)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%s: %w", op, repository.ErrNotFound)
	}

	return nil
}

// TouchAPIKey records the last use of the key.
func (r *APIKeysRepository) TouchAPIKey(ctx context.Context, id int, at time.Time) error {
	const op = "storage.postgres.TouchAPIKey"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "UPDATE api_keys SET last_used_at = $2 WHERE api_key_id = $1", id, at); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
