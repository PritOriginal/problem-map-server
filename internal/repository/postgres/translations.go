package postgres

import "fmt"

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
