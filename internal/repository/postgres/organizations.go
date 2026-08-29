package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
)

// OrganizationsRepository stores city services, their members and
// responsibilities, and the assignment of marks to them.
type OrganizationsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewOrganizations(db *sqlx.DB, c *trmsqlx.CtxGetter) *OrganizationsRepository {
	return &OrganizationsRepository{
		db:     db,
		getter: c,
	}
}

const organizationColumns = "organization_id, name, description, created_at"

func (r *OrganizationsRepository) AddOrganization(ctx context.Context, org models.Organization) (int64, error) {
	const op = "storage.postgres.AddOrganization"

	var id int64
	query := "INSERT INTO organizations (name, description) VALUES ($1, $2) RETURNING organization_id"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, org.Name, org.Description); err != nil {
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

// UpdateOrganization changes the given fields (nil keeps the current value).
func (r *OrganizationsRepository) UpdateOrganization(ctx context.Context, id int, upd models.OrganizationUpdate) error {
	const op = "storage.postgres.UpdateOrganization"

	query := `
		UPDATE organizations SET
			name = COALESCE($2, name),
			description = COALESCE($3, description)
		WHERE organization_id = $1
		`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, id, upd.Name, upd.Description)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *OrganizationsRepository) GetOrganizations(ctx context.Context) ([]models.OrganizationRef, error) {
	const op = "storage.postgres.GetOrganizations"

	orgs := []models.OrganizationRef{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &orgs, "SELECT organization_id, name FROM organizations ORDER BY name, organization_id"); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return orgs, nil
}

func (r *OrganizationsRepository) GetOrganizationById(ctx context.Context, id int) (models.Organization, error) {
	const op = "storage.postgres.GetOrganizationById"

	var org models.Organization
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &org, "SELECT "+organizationColumns+" FROM organizations WHERE organization_id = $1", id); err != nil {
		return org, wrapPgError(op, err)
	}

	return org, nil
}

// GetOrganizationByUserId returns the organization the user is a member of
// (repository.ErrNotFound when the user belongs to none).
func (r *OrganizationsRepository) GetOrganizationByUserId(ctx context.Context, userId int) (models.Organization, error) {
	const op = "storage.postgres.GetOrganizationByUserId"

	query := `
		SELECT o.organization_id, o.name, o.description, o.created_at
		FROM organizations o
		JOIN organization_members m ON m.organization_id = o.organization_id
		WHERE m.user_id = $1
		`

	var org models.Organization
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &org, query, userId); err != nil {
		return org, wrapPgError(op, err)
	}

	return org, nil
}

// AddMember adds the user to the organization. A user already in an
// organization (this or another one) yields repository.ErrExists; an
// unknown organization or user repository.ErrInvalidReference.
func (r *OrganizationsRepository) AddMember(ctx context.Context, orgId, userId int) error {
	const op = "storage.postgres.AddMember"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, "INSERT INTO organization_members (organization_id, user_id) VALUES ($1, $2)", orgId, userId); err != nil {
		return wrapPgError(op, err)
	}

	return nil
}

// RemoveMember removes the user from the organization
// (repository.ErrNotFound when they are not a member).
func (r *OrganizationsRepository) RemoveMember(ctx context.Context, orgId, userId int) error {
	const op = "storage.postgres.RemoveMember"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2", orgId, userId)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// GetMembers returns the members of the organization (public fields only).
func (r *OrganizationsRepository) GetMembers(ctx context.Context, orgId int) ([]models.User, error) {
	const op = "storage.postgres.GetMembers"

	query := `
		SELECT u.user_id, u.name, u.rating, u.role
		FROM users u
		JOIN organization_members m ON m.user_id = u.user_id
		WHERE m.organization_id = $1
		ORDER BY u.user_id
		`

	users := []models.User{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &users, query, orgId); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return users, nil
}

// GetMemberIDs returns the ids of the organization's members (for notifications).
func (r *OrganizationsRepository) GetMemberIDs(ctx context.Context, orgId int) ([]int, error) {
	const op = "storage.postgres.GetMemberIDs"

	ids := []int{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &ids, "SELECT user_id FROM organization_members WHERE organization_id = $1 ORDER BY user_id", orgId); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return ids, nil
}

// IsMember reports whether the user belongs to the organization.
func (r *OrganizationsRepository) IsMember(ctx context.Context, orgId, userId int) (bool, error) {
	const op = "storage.postgres.IsMember"

	var ok bool
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &ok, "SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id = $1 AND user_id = $2)", orgId, userId); err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	return ok, nil
}

// AddResponsibility registers the (type, boundary) pair; a duplicate yields
// repository.ErrExists, an unknown organization / type / boundary
// repository.ErrInvalidReference.
func (r *OrganizationsRepository) AddResponsibility(ctx context.Context, resp models.OrganizationResponsibility) (int64, error) {
	const op = "storage.postgres.AddResponsibility"

	var id int64
	query := "INSERT INTO organization_responsibilities (organization_id, mark_type_id, boundary_id) VALUES ($1, $2, $3) RETURNING id"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, resp.OrganizationID, resp.MarkTypeID, resp.BoundaryID); err != nil {
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

// RemoveResponsibility deletes the pair (repository.ErrNotFound when absent).
func (r *OrganizationsRepository) RemoveResponsibility(ctx context.Context, resp models.OrganizationResponsibility) error {
	const op = "storage.postgres.RemoveResponsibility"

	query := "DELETE FROM organization_responsibilities WHERE organization_id = $1 AND mark_type_id = $2 AND boundary_id = $3"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, resp.OrganizationID, resp.MarkTypeID, resp.BoundaryID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *OrganizationsRepository) GetResponsibilities(ctx context.Context, orgId int) ([]models.OrganizationResponsibility, error) {
	const op = "storage.postgres.GetResponsibilities"

	query := "SELECT id, organization_id, mark_type_id, boundary_id FROM organization_responsibilities WHERE organization_id = $1 ORDER BY id"

	resps := []models.OrganizationResponsibility{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &resps, query, orgId); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return resps, nil
}

// FindResponsibleOrganization returns the organization responsible for the
// mark: one with a responsibility for the mark's type whose boundary
// contains the mark. When several boundaries match, the most local one
// (highest admin_level) wins; repository.ErrNotFound when there is none.
func (r *OrganizationsRepository) FindResponsibleOrganization(ctx context.Context, markId int) (models.Organization, error) {
	const op = "storage.postgres.FindResponsibleOrganization"

	query := `
		SELECT o.organization_id, o.name, o.description, o.created_at
		FROM marks m
		JOIN organization_responsibilities resp ON resp.mark_type_id = m.type_mark_id
		JOIN admin_boundaries b ON b.id = resp.boundary_id AND ST_Contains(b.geom, m.geom)
		JOIN organizations o ON o.organization_id = resp.organization_id
		WHERE m.mark_id = $1
		ORDER BY b.admin_level DESC, resp.id ASC
		LIMIT 1
		`

	var org models.Organization
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &org, query, markId); err != nil {
		return org, wrapPgError(op, err)
	}

	return org, nil
}

// AssignMark sets the organization of the mark and its SLA deadline
// (now + types_marks.sla_hours), clears the reported breach and returns
// the deadline.
func (r *OrganizationsRepository) AssignMark(ctx context.Context, markId, orgId int) (time.Time, error) {
	const op = "storage.postgres.AssignMark"

	query := `
		UPDATE marks m SET
			organization_id = $2,
			sla_due_at = NOW() + make_interval(hours => t.sla_hours),
			sla_breached_at = NULL,
			updated_at = NOW()
		FROM types_marks t
		WHERE m.mark_id = $1 AND t.type_mark_id = m.type_mark_id
		RETURNING m.sla_due_at
		`

	var dueAt time.Time
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &dueAt, query, markId, orgId); err != nil {
		return time.Time{}, wrapPgError(op, err)
	}

	return dueAt, nil
}

// GetOrganizationMarks returns the organization's queue: overdue marks
// first, then by the nearest deadline.
func (r *OrganizationsRepository) GetOrganizationMarks(ctx context.Context, orgId int, filters models.GetOrganizationMarksFilters) (models.Page[models.Mark], error) {
	const op = "storage.postgres.GetOrganizationMarks"

	q := newListQuery(markColumns, "marks").
		Where("marks.organization_id = ?", orgId).
		OrderBy("is_overdue DESC, sla_due_at ASC NULLS LAST, marks.mark_id ASC").
		Paginate(filters.Pagination)
	q.ColumnArgs(models.ViewerFromContext(ctx))

	if len(filters.MarkStatusIds) > 0 {
		q.Where("mark_status_id IN (?)", filters.MarkStatusIds)
	}
	if filters.Overdue {
		q.Where("marks.sla_due_at < NOW() AND mark_status_id IN (?)", models.SLAStatuses())
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Mark](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// GetOverdueMarks returns every assigned mark whose SLA deadline passed
// before now while it is still confirmed or in progress and whose breach
// has not been reported yet (see MarkSLABreached).
func (r *OrganizationsRepository) GetOverdueMarks(ctx context.Context, now time.Time) ([]models.Mark, error) {
	const op = "storage.postgres.GetOverdueMarks"

	q := newListQuery(markColumns, "marks").
		Where("marks.organization_id IS NOT NULL").
		Where("marks.sla_due_at < ?", now).
		Where("marks.sla_breached_at IS NULL").
		Where("mark_status_id IN (?)", models.SLAStatuses()).
		OrderBy("sla_due_at ASC, marks.mark_id ASC")
	q.ColumnArgs(models.ViewerFromContext(ctx))

	query, args, err := q.selectQuery()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	marks := []models.Mark{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &marks, query, args...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return marks, nil
}

// MarkSLABreached records that the breach of the deadline dueAt was
// reported. A mark whose deadline was reset meanwhile is left untouched, so
// the new deadline is checked again.
func (r *OrganizationsRepository) MarkSLABreached(ctx context.Context, markId int, dueAt time.Time) error {
	const op = "storage.postgres.MarkSLABreached"

	query := "UPDATE marks SET sla_breached_at = NOW() WHERE mark_id = $1 AND sla_due_at = $2 AND sla_breached_at IS NULL"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if _, err := tr.ExecContext(ctx, query, markId, dueAt); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
