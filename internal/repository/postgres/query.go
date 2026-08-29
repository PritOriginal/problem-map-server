package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
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
	var sb strings.Builder
	sb.WriteString("SELECT ")
	sb.WriteString(q.columns)
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

// selectPage runs the item query and, when a window was requested, a
// separate count over the same conditions. The count is skipped when the
// page itself proves the total: no window at all, or a non-empty page
// shorter than the limit (the last one).
func selectPage[T any](ctx context.Context, tr trmsqlx.Tr, q *listQuery) (models.Page[T], error) {
	page := models.Page[T]{Items: []T{}}

	query, args, err := q.selectQuery()
	if err != nil {
		return page, err
	}
	if err := tr.SelectContext(ctx, &page.Items, query, args...); err != nil {
		return page, err
	}

	n := len(page.Items)
	switch {
	case q.page.Limit == 0:
		page.Total = n
		return page, nil
	case n < q.page.Limit && (n > 0 || q.page.Offset == 0):
		page.Total = q.page.Offset + n
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
