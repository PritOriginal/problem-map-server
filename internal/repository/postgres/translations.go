package postgres

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

// Dictionary names are stored in the translations table keyed by entity and
// row id. The helpers below build the SQL fragments shared by the dictionary
// queries; the queries bind the requested language as $1 and the fallback
// language as $2.

// translationJoins joins the requested (tr) and fallback (fb) translations
// of the entity identified by entity and the id column expression idCol.
func translationJoins(entity, idCol string) string {
	return fmt.Sprintf(`LEFT JOIN translations tr ON tr.entity = '%[1]s' AND tr.entity_id = %[2]s AND tr.lang = $1
		LEFT JOIN translations fb ON fb.entity = '%[1]s' AND fb.entity_id = %[2]s AND fb.lang = $2`, entity, idCol)
}

// translatedName selects the localised name: the requested language, then
// the fallback language, then the raw column.
func translatedName(rawCol string) string {
	return fmt.Sprintf("COALESCE(tr.name, fb.name, %s) AS name", rawCol)
}

// SetTranslation stores (or replaces) the name of an entity row in lang.
func (r *MarksRepository) SetTranslation(ctx context.Context, entity string, entityId int, lang models.Lang, name string) error {
	const op = "storage.postgres.SetTranslation"

	query := `INSERT INTO translations (entity, entity_id, lang, name) VALUES ($1, $2, $3, $4)
		ON CONFLICT (entity, entity_id, lang) DO UPDATE SET name = EXCLUDED.name`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, query, entity, entityId, lang, name); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
