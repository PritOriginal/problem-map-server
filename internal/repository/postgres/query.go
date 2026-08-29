package postgres

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
)

// listQuery assembles a SELECT with optional conditions, ordering and a
// limit/offset window. Conditions use "?" placeholders; slice arguments are
// expanded with sqlx.In and everything is rebound to $n before execution,
// so callers never number placeholders by hand.
type listQuery struct {
	columns    string
	columnArgs []any
	from       string
	conds      []string
	args       []any
	orderBy    string
	page       models.Pagination
}

func newListQuery(columns, from string) *listQuery {
	return &listQuery{columns: columns, from: from}
}

// ColumnArgs sets the arguments for placeholders used in the column list
// (e.g. a computed distance). They are excluded from the count query.
func (q *listQuery) ColumnArgs(args ...any) *listQuery {
	q.columnArgs = args
	return q
}

// Where adds a condition joined with AND. Use "col IN (?)" with a slice
// argument for IN-lists.
func (q *listQuery) Where(cond string, args ...any) *listQuery {
	q.conds = append(q.conds, cond)
	q.args = append(q.args, args...)
	return q
}

// OrderBy sets the ORDER BY clause (without the keyword). Column names must
// come from a whitelist, never from user input.
func (q *listQuery) OrderBy(orderBy string) *listQuery {
	q.orderBy = orderBy
	return q
}

// Paginate sets the LIMIT/OFFSET window; a zero limit disables it.
func (q *listQuery) Paginate(p models.Pagination) *listQuery {
	q.page = p
	return q
}

func (q *listQuery) where() string {
	if len(q.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(q.conds, " AND ")
}

// selectQuery renders the item query with placeholders rebound to $n.
func (q *listQuery) selectQuery() (string, []any, error) {
	return q.render(false)
}

// pageQuery renders the item query with an extra trailing "total" column:
// COUNT(*) OVER() is evaluated before LIMIT/OFFSET, so every row carries
// the number of rows matching the conditions.
func (q *listQuery) pageQuery() (string, []any, error) {
	return q.render(true)
}

func (q *listQuery) render(withTotal bool) (string, []any, error) {
	var sb strings.Builder
	sb.WriteString("SELECT ")
	sb.WriteString(q.columns)
	if withTotal {
		sb.WriteString(", COUNT(*) OVER() AS total")
	}
	sb.WriteString(" FROM ")
	sb.WriteString(q.from)
	sb.WriteString(q.where())
	if q.orderBy != "" {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(q.orderBy)
	}
	args := make([]any, 0, len(q.columnArgs)+len(q.args)+2)
	args = append(args, q.columnArgs...)
	args = append(args, q.args...)
	if q.page.Limit > 0 {
		sb.WriteString(" LIMIT ? OFFSET ?")
		args = append(args, q.page.Limit, q.page.Offset)
	}
	return bind(sb.String(), args)
}

// countQuery renders "SELECT COUNT(*)" over the same FROM/WHERE.
func (q *listQuery) countQuery() (string, []any, error) {
	return bind("SELECT COUNT(*) FROM "+q.from+q.where(), q.args)
}

// bind expands slice arguments (sqlx.In) and rebinds "?" to "$n".
func bind(query string, args []any) (string, []any, error) {
	query, args, err := sqlx.In(query, args...)
	if err != nil {
		return "", nil, fmt.Errorf("bind query: %w", err)
	}
	return sqlx.Rebind(sqlx.DOLLAR, query), args, nil
}

// selectPage runs the item query with the total carried by every row
// (COUNT(*) OVER()), so a page costs one round trip. Only an empty page
// beyond the first (offset > 0) carries no rows to read the total from and
// needs a separate count.
func selectPage[T any](ctx context.Context, tr trmsqlx.Tr, q *listQuery) (models.Page[T], error) {
	page := models.Page[T]{Items: []T{}}

	query, args, err := q.pageQuery()
	if err != nil {
		return page, err
	}

	total, err := scanPage(ctx, tr, query, args, &page.Items)
	if err != nil {
		return page, err
	}

	switch {
	case len(page.Items) > 0:
		page.Total = total
		return page, nil
	case q.page.Limit == 0 || q.page.Offset == 0:
		return page, nil
	}

	countQuery, countArgs, err := q.countQuery()
	if err != nil {
		return page, err
	}
	if err := tr.GetContext(ctx, &page.Total, countQuery, countArgs...); err != nil {
		return page, err
	}
	return page, nil
}

// scanPage scans rows into *items the way sqlx.Rows.StructScan does (db
// tags, embedded structs, nil pointers allocated), except that the last
// column, total, goes to the returned int instead of a struct field.
func scanPage[T any](ctx context.Context, tr trmsqlx.Tr, query string, args []any, items *[]T) (int, error) {
	rows, err := tr.QueryxContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	last := len(cols) - 1
	if last < 1 || cols[last] != "total" {
		return 0, fmt.Errorf("scanPage: total column missing in %q", query)
	}

	itemType := reflect.TypeFor[T]()
	traversals := rows.Mapper.TraversalsByName(itemType, cols[:last])
	for i, t := range traversals {
		if len(t) == 0 {
			return 0, fmt.Errorf("scanPage: missing destination name %s in %s", cols[i], itemType)
		}
	}

	var total int
	dest := make([]any, len(cols))
	dest[last] = &total
	for rows.Next() {
		var item T
		v := reflect.ValueOf(&item).Elem()
		for i, t := range traversals {
			dest[i] = reflectx.FieldByIndexes(v, t).Addr().Interface()
		}
		if err := rows.Scan(dest...); err != nil {
			return 0, err
		}
		*items = append(*items, item)
	}

	return total, rows.Err()
}
