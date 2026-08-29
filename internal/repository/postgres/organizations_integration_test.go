//go:build integration

package postgres_test

import (
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
)

// seedOrganization creates an organization and returns its id.
func (s *PostgresSuite) seedOrganization(name string) int {
	id, err := s.organizations.AddOrganization(s.ctx, models.Organization{Name: name, Description: "test"})
	s.Require().NoError(err)
	return int(id)
}

// seedBoundary inserts an admin boundary covering the whole fixture area
// with the given admin level and returns its id.
func (s *PostgresSuite) seedBoundary(name string, adminLevel int) int {
	var id int
	s.Require().NoError(s.db.GetContext(s.ctx, &id, `
		INSERT INTO admin_boundaries (osm_id, name, admin_level, geom)
		VALUES ($1, $2, $3, ST_SetSRID(ST_Multi(ST_MakeEnvelope(41.0, 52.0, 42.0, 53.0)), 4326))
		RETURNING id
	`, 2000+adminLevel, name, adminLevel))
	return id
}

func (s *PostgresSuite) TestOrganizations_FindResponsibleOrganization() {
	// "Центр" (level 8) contains marks 1 (type 1) and 2 (type 2); mark 3 is
	// outside every fixture boundary.
	central := s.seedOrganization("Центральная служба")
	_, err := s.organizations.AddResponsibility(s.ctx, models.OrganizationResponsibility{OrganizationID: central, MarkTypeID: 2, BoundaryID: fxBoundaryMain})
	s.Require().NoError(err)

	// The city-wide service (level 4 boundary) handles type 1 and type 2
	// everywhere; the more local "Центр" must still win for mark 2.
	cityBoundary := s.seedBoundary("Город", 4)
	city := s.seedOrganization("Городская служба")
	for _, typeID := range []int{1, 2} {
		_, err := s.organizations.AddResponsibility(s.ctx, models.OrganizationResponsibility{OrganizationID: city, MarkTypeID: typeID, BoundaryID: cityBoundary})
		s.Require().NoError(err)
	}

	// A duplicate pair is rejected.
	_, err = s.organizations.AddResponsibility(s.ctx, models.OrganizationResponsibility{OrganizationID: city, MarkTypeID: 1, BoundaryID: cityBoundary})
	s.ErrorIs(err, repository.ErrExists)
	// An unknown boundary is an invalid reference.
	_, err = s.organizations.AddResponsibility(s.ctx, models.OrganizationResponsibility{OrganizationID: city, MarkTypeID: 1, BoundaryID: 999})
	s.ErrorIs(err, repository.ErrInvalidReference)

	tests := []struct {
		name    string
		markID  int
		wantOrg int
		wantErr error
	}{
		{name: "most local boundary wins", markID: fxMarkInside, wantOrg: central},
		{name: "type without local service falls back to the city", markID: fxMarkNear, wantOrg: city},
		{name: "outside local boundary but inside the city", markID: fxMarkFar, wantOrg: city},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.organizations.FindResponsibleOrganization(s.ctx, tt.markID)
			s.Require().NoError(err)
			s.Equal(tt.wantOrg, got.ID)
		})
	}

	// Without the city-wide responsibilities nothing covers mark 3.
	for _, typeID := range []int{1, 2} {
		s.Require().NoError(s.organizations.RemoveResponsibility(s.ctx, models.OrganizationResponsibility{OrganizationID: city, MarkTypeID: typeID, BoundaryID: cityBoundary}))
	}
	_, err = s.organizations.FindResponsibleOrganization(s.ctx, fxMarkFar)
	s.ErrorIs(err, repository.ErrNotFound)
	_, err = s.organizations.FindResponsibleOrganization(s.ctx, 999)
	s.ErrorIs(err, repository.ErrNotFound)
}

func (s *PostgresSuite) TestOrganizations_AssignMark() {
	org := s.seedOrganization("Служба")

	// The type's SLA defines the deadline.
	_, err := s.db.ExecContext(s.ctx, "UPDATE types_marks SET sla_hours = 24 WHERE type_mark_id = 2")
	s.Require().NoError(err)

	before := time.Now()
	dueAt, err := s.organizations.AssignMark(s.ctx, fxMarkInside, org)
	s.Require().NoError(err)
	s.WithinDuration(before.Add(24*time.Hour), dueAt, time.Minute)

	mark, err := s.marks.GetMarkById(s.ctx, fxMarkInside)
	s.Require().NoError(err)
	s.Equal(int64(org), mark.OrganizationID.Int64)
	s.True(mark.SLADueAt.Valid)
	s.WithinDuration(dueAt, mark.SLADueAt.Time, time.Second)
	s.False(mark.IsOverdue)

	_, err = s.organizations.AssignMark(s.ctx, 999, org)
	s.ErrorIs(err, repository.ErrNotFound)
	_, err = s.organizations.AssignMark(s.ctx, fxMarkInside, 999)
	s.ErrorIs(err, repository.ErrInvalidReference)
}

func (s *PostgresSuite) TestOrganizations_Queue() {
	org := s.seedOrganization("Служба")
	other := s.seedOrganization("Другая")

	// Four marks of the organization with different deadlines and statuses,
	// plus one of another organization that must never show up:
	//   A: confirmed, overdue by 2 h
	//   B: in progress, overdue by 1 day (the most overdue)
	//   C: confirmed, due in 1 h
	//   D: under review, deadline passed -> not overdue (SLA not running),
	//      but still sorted by its deadline among the non-overdue marks
	//   E: other organization
	ins := func(status models.MarkStatusType, orgID int, due string) int {
		var id int
		s.Require().NoError(s.db.GetContext(s.ctx, &id, `
			INSERT INTO marks (description, geom, type_mark_id, user_id, mark_status_id, organization_id, sla_due_at)
			VALUES ('q', ST_SetSRID(ST_MakePoint(41.41, 52.70), 4326), 1, 1, $1, $2, NOW() + $3::interval)
			RETURNING mark_id
		`, status, orgID, due))
		return id
	}
	a := ins(models.ConfirmedStatus, org, "-2 hours")
	b := ins(models.InProgressStatus, org, "-1 day")
	c := ins(models.ConfirmedStatus, org, "1 hour")
	d := ins(models.UnderReviewStatus, org, "-3 hours")
	ins(models.ConfirmedStatus, other, "-5 hours")

	tests := []struct {
		name    string
		filters models.GetOrganizationMarksFilters
		wantIDs []int
	}{
		{name: "overdue first, then nearest deadline", wantIDs: []int{b, a, d, c}},
		{name: "overdue only", filters: models.GetOrganizationMarksFilters{Overdue: true}, wantIDs: []int{b, a}},
		{name: "status filter", filters: models.GetOrganizationMarksFilters{MarkStatusIds: []int{int(models.ConfirmedStatus)}}, wantIDs: []int{a, c}},
		{name: "pagination", filters: models.GetOrganizationMarksFilters{Pagination: models.Pagination{Limit: 2, Offset: 1}}, wantIDs: []int{a, d}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.organizations.GetOrganizationMarks(s.ctx, org, tt.filters)
			s.Require().NoError(err)
			s.Equal(tt.wantIDs, ids(page.Items, func(m models.Mark) int { return m.ID }))
			if tt.filters.Pagination.Limit == 0 {
				s.Equal(len(tt.wantIDs), page.Total)
			}
			for _, m := range page.Items {
				s.Equal(m.ID == a || m.ID == b, m.IsOverdue, "mark %d", m.ID)
			}
		})
	}

	// The SLA check sees the overdue marks of every organization.
	overdue, err := s.organizations.GetOverdueMarks(s.ctx, time.Now())
	s.Require().NoError(err)
	s.Len(overdue, 3)
	s.Equal(b, overdue[0].ID)
}

func (s *PostgresSuite) TestOrganizations_Members() {
	org := s.seedOrganization("Служба")

	s.Require().NoError(s.organizations.AddMember(s.ctx, org, fxUserAlice))
	s.ErrorIs(s.organizations.AddMember(s.ctx, org, fxUserAlice), repository.ErrExists)
	s.ErrorIs(s.organizations.AddMember(s.ctx, 999, fxUserBob), repository.ErrInvalidReference)

	got, err := s.organizations.GetOrganizationByUserId(s.ctx, fxUserAlice)
	s.Require().NoError(err)
	s.Equal(org, got.ID)
	_, err = s.organizations.GetOrganizationByUserId(s.ctx, fxUserBob)
	s.ErrorIs(err, repository.ErrNotFound)

	ok, err := s.organizations.IsMember(s.ctx, org, fxUserAlice)
	s.Require().NoError(err)
	s.True(ok)
	ok, err = s.organizations.IsMember(s.ctx, org, fxUserBob)
	s.Require().NoError(err)
	s.False(ok)

	members, err := s.organizations.GetMembers(s.ctx, org)
	s.Require().NoError(err)
	s.Equal([]int{fxUserAlice}, ids(members, func(u models.User) int { return u.Id }))
	memberIDs, err := s.organizations.GetMemberIDs(s.ctx, org)
	s.Require().NoError(err)
	s.Equal([]int{fxUserAlice}, memberIDs)

	s.Require().NoError(s.organizations.RemoveMember(s.ctx, org, fxUserAlice))
	s.ErrorIs(s.organizations.RemoveMember(s.ctx, org, fxUserAlice), repository.ErrNotFound)

	// The service role is accepted by the users.role check.
	s.Require().NoError(s.users.UpdateRole(s.ctx, fxUserAlice, models.RoleService))
	user, err := s.users.GetUserById(s.ctx, fxUserAlice)
	s.Require().NoError(err)
	s.Equal(models.RoleService, user.Role)
}

func (s *PostgresSuite) TestOrganizations_CRUD() {
	id := s.seedOrganization("Служба")

	name := "Новое имя"
	s.Require().NoError(s.organizations.UpdateOrganization(s.ctx, id, models.OrganizationUpdate{Name: &name}))
	s.ErrorIs(s.organizations.UpdateOrganization(s.ctx, 999, models.OrganizationUpdate{Name: &name}), repository.ErrNotFound)

	got, err := s.organizations.GetOrganizationById(s.ctx, id)
	s.Require().NoError(err)
	s.Equal(name, got.Name)
	s.Equal("test", got.Description)

	list, err := s.organizations.GetOrganizations(s.ctx)
	s.Require().NoError(err)
	s.Equal([]models.OrganizationRef{{ID: id, Name: name}}, list)

	// The new status is present with its parent.
	statuses, err := s.marks.GetMarkStatuses(s.ctx)
	s.Require().NoError(err)
	var found bool
	for _, st := range statuses {
		if st.ID == int(models.InProgressStatus) {
			found = true
			s.Equal("В работе", st.Name)
			s.Equal(int64(models.ConfirmedStatus), st.ParentId.Int64)
		}
	}
	s.True(found, "status 'В работе' seeded by the migration")
}

func (s *PostgresSuite) TestAnalytics_KPIByOrganization() {
	org := s.seedOrganization("Служба")
	_, err := s.organizations.AssignMark(s.ctx, fxMarkInside, org)
	s.Require().NoError(err)
	_, err = s.db.ExecContext(s.ctx, "UPDATE marks SET sla_due_at = NOW() - INTERVAL '1 hour' WHERE mark_id = $1", fxMarkInside)
	s.Require().NoError(err)

	kpi, err := s.analytics.GetKPI(s.ctx, models.AnalyticsFilters{})
	s.Require().NoError(err)
	s.Equal([]models.OrganizationKPI{{OrganizationID: org, Name: "Служба", Total: 1, Overdue: 1}}, kpi.ByOrganization)
	s.InDelta(1.0, kpi.SLABreachShare, 1e-9)
}
