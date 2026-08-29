package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

type OrganizationsRepository interface {
	AddOrganization(ctx context.Context, org models.Organization) (int64, error)
	UpdateOrganization(ctx context.Context, id int, upd models.OrganizationUpdate) error
	GetOrganizations(ctx context.Context) ([]models.OrganizationRef, error)
	GetOrganizationById(ctx context.Context, id int) (models.Organization, error)
	GetOrganizationByUserId(ctx context.Context, userId int) (models.Organization, error)
	AddMember(ctx context.Context, orgId, userId int) error
	RemoveMember(ctx context.Context, orgId, userId int) error
	GetMembers(ctx context.Context, orgId int) ([]models.User, error)
	GetMemberIDs(ctx context.Context, orgId int) ([]int, error)
	IsMember(ctx context.Context, orgId, userId int) (bool, error)
	AddResponsibility(ctx context.Context, resp models.OrganizationResponsibility) (int64, error)
	RemoveResponsibility(ctx context.Context, resp models.OrganizationResponsibility) error
	GetResponsibilities(ctx context.Context, orgId int) ([]models.OrganizationResponsibility, error)
	// FindResponsibleOrganization returns the organization whose
	// responsibility (type + boundary containing the mark) matches the mark;
	// the most local boundary wins (repository.ErrNotFound when none).
	FindResponsibleOrganization(ctx context.Context, markId int) (models.Organization, error)
	// AssignMark sets organization_id and sla_due_at (now + the type's SLA)
	// and returns the deadline.
	AssignMark(ctx context.Context, markId, orgId int) (time.Time, error)
	GetOrganizationMarks(ctx context.Context, orgId int, filters models.GetOrganizationMarksFilters) (models.Page[models.Mark], error)
	GetOverdueMarks(ctx context.Context, now time.Time) ([]models.Mark, error)
}

type OrganizationsRepositories struct {
	Organizations OrganizationsRepository
	Marks         MarksRepository
	Checks        ChecksRepository
	Photos        PhotosRepository
	Users         UsersRepository
	// RefreshTokens / AuthVersions are optional (nil without Redis): a
	// role change revokes the user's sessions so the new role applies.
	RefreshTokens RefreshStore
	AuthVersions  AuthVersionStore
}

// Organizations manages city services: membership, responsibilities, the
// assignment of confirmed marks (automatic and manual) and the work of the
// services on their queue (start / resolve).
type Organizations struct {
	log       *slog.Logger
	trManager trm.Manager
	repos     OrganizationsRepositories
	sessions  sessions
	events    events.Publisher
}

func NewOrganizations(log *slog.Logger, trManager trm.Manager, repos OrganizationsRepositories) *Organizations {
	return &Organizations{
		log:       log,
		trManager: trManager,
		repos:     repos,
		sessions:  sessions{log: log, refresh: repos.RefreshTokens, versions: repos.AuthVersions},
		events:    events.NoopPublisher{},
	}
}

// WithEvents sets the publisher of mark.assigned / mark.status_changed
// events. Without it events are dropped.
func (uc *Organizations) WithEvents(p events.Publisher) *Organizations {
	if p != nil {
		uc.events = p
	}
	return uc
}

func (uc *Organizations) Create(ctx context.Context, org models.Organization) (models.Organization, error) {
	const op = "usecase.Organizations.Create"

	id, err := uc.repos.Organizations.AddOrganization(ctx, org)
	if err != nil {
		return models.Organization{}, mapRepoErr(op, err)
	}

	created, err := uc.repos.Organizations.GetOrganizationById(ctx, int(id))
	if err != nil {
		return models.Organization{}, mapRepoErr(op, err)
	}
	return created, nil
}

func (uc *Organizations) Update(ctx context.Context, id int, upd models.OrganizationUpdate) (models.Organization, error) {
	const op = "usecase.Organizations.Update"

	if upd.IsEmpty() {
		return models.Organization{}, fmt.Errorf("%s: %w: nothing to update", op, ErrInvalidArgument)
	}
	if err := uc.repos.Organizations.UpdateOrganization(ctx, id, upd); err != nil {
		return models.Organization{}, mapRepoErr(op, err)
	}

	org, err := uc.repos.Organizations.GetOrganizationById(ctx, id)
	if err != nil {
		return models.Organization{}, mapRepoErr(op, err)
	}
	return org, nil
}

// List returns every organization (public ids and names).
func (uc *Organizations) List(ctx context.Context) ([]models.OrganizationRef, error) {
	const op = "usecase.Organizations.List"

	orgs, err := uc.repos.Organizations.GetOrganizations(ctx)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}
	return orgs, nil
}

// Get returns the organization with its members and responsibilities.
func (uc *Organizations) Get(ctx context.Context, id int) (models.OrganizationDetails, error) {
	const op = "usecase.Organizations.Get"

	org, err := uc.repos.Organizations.GetOrganizationById(ctx, id)
	if err != nil {
		return models.OrganizationDetails{}, mapRepoErr(op, err)
	}
	return uc.details(ctx, op, org)
}

// GetMine returns the organization the user is a member of.
func (uc *Organizations) GetMine(ctx context.Context, userId int) (models.OrganizationDetails, error) {
	const op = "usecase.Organizations.GetMine"

	org, err := uc.repos.Organizations.GetOrganizationByUserId(ctx, userId)
	if err != nil {
		return models.OrganizationDetails{}, mapRepoErr(op, err)
	}
	return uc.details(ctx, op, org)
}

func (uc *Organizations) details(ctx context.Context, op string, org models.Organization) (models.OrganizationDetails, error) {
	members, err := uc.repos.Organizations.GetMembers(ctx, org.ID)
	if err != nil {
		return models.OrganizationDetails{}, mapRepoErr(op, err)
	}
	for i := range members {
		members[i] = members[i].Public()
	}

	resps, err := uc.repos.Organizations.GetResponsibilities(ctx, org.ID)
	if err != nil {
		return models.OrganizationDetails{}, mapRepoErr(op, err)
	}

	return models.OrganizationDetails{Organization: org, Members: members, Responsibilities: resps}, nil
}

// AddMember adds the user to the organization and gives them the service
// role. Only plain users (or service users without an organization) may be
// added: moderators and admins keep their role (ErrConflict). A user who is
// already a member of an organization yields ErrConflict too.
func (uc *Organizations) AddMember(ctx context.Context, orgId, userId int) error {
	const op = "usecase.Organizations.AddMember"

	if _, err := uc.repos.Organizations.GetOrganizationById(ctx, orgId); err != nil {
		return mapRepoErr(op, err)
	}
	user, err := uc.repos.Users.GetUserById(ctx, userId)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if user.Role != models.RoleUser && user.Role != models.RoleService {
		return fmt.Errorf("%s: %w: user has role %q", op, ErrConflict, user.Role)
	}

	err = uc.trManager.Do(ctx, func(ctx context.Context) error {
		if err := uc.repos.Organizations.AddMember(ctx, orgId, userId); err != nil {
			return err
		}
		return uc.repos.Users.UpdateRole(ctx, userId, models.RoleService)
	})
	if err != nil {
		return mapRepoErr(op, err)
	}

	// Tokens issued with the old role stop working, so the member gets the
	// service role on the next sign-in / refresh.
	uc.sessions.revokeAll(ctx, op, userId)
	uc.log.Info("organization member added", slog.Int("organization_id", orgId), slog.Int("user_id", userId))

	return nil
}

// RemoveMember removes the user from the organization and returns them to
// the plain user role.
func (uc *Organizations) RemoveMember(ctx context.Context, orgId, userId int) error {
	const op = "usecase.Organizations.RemoveMember"

	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		if err := uc.repos.Organizations.RemoveMember(ctx, orgId, userId); err != nil {
			return err
		}
		return uc.repos.Users.UpdateRole(ctx, userId, models.RoleUser)
	})
	if err != nil {
		return mapRepoErr(op, err)
	}

	uc.sessions.revokeAll(ctx, op, userId)
	uc.log.Info("organization member removed", slog.Int("organization_id", orgId), slog.Int("user_id", userId))

	return nil
}

func (uc *Organizations) AddResponsibility(ctx context.Context, resp models.OrganizationResponsibility) (models.OrganizationResponsibility, error) {
	const op = "usecase.Organizations.AddResponsibility"

	if err := resp.Validate(); err != nil {
		return models.OrganizationResponsibility{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if _, err := uc.repos.Organizations.GetOrganizationById(ctx, resp.OrganizationID); err != nil {
		return models.OrganizationResponsibility{}, mapRepoErr(op, err)
	}

	id, err := uc.repos.Organizations.AddResponsibility(ctx, resp)
	if err != nil {
		return models.OrganizationResponsibility{}, mapRepoErr(op, err)
	}
	resp.ID = int(id)
	return resp, nil
}

func (uc *Organizations) RemoveResponsibility(ctx context.Context, resp models.OrganizationResponsibility) error {
	const op = "usecase.Organizations.RemoveResponsibility"

	if err := resp.Validate(); err != nil {
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if err := uc.repos.Organizations.RemoveResponsibility(ctx, resp); err != nil {
		return mapRepoErr(op, err)
	}
	return nil
}

// ListMarks returns the organization's queue. Admins see any queue,
// service users only their own organization's (ErrForbidden otherwise).
func (uc *Organizations) ListMarks(ctx context.Context, actor models.Actor, orgId int, filters models.GetOrganizationMarksFilters) (models.Page[models.Mark], error) {
	const op = "usecase.Organizations.ListMarks"

	if err := filters.Validate(); err != nil {
		return models.Page[models.Mark]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if err := uc.authorizeOrganization(ctx, actor, orgId); err != nil {
		return models.Page[models.Mark]{}, fmt.Errorf("%s: %w", op, err)
	}

	page, err := uc.repos.Organizations.GetOrganizationMarks(ctx, orgId, filters)
	if err != nil {
		return page, mapRepoErr(op, err)
	}
	return page, nil
}

// authorizeOrganization allows admins and members of the organization.
func (uc *Organizations) authorizeOrganization(ctx context.Context, actor models.Actor, orgId int) error {
	if actor.Role == models.RoleAdmin {
		if _, err := uc.repos.Organizations.GetOrganizationById(ctx, orgId); err != nil {
			return mapRepoErr("authorize", err)
		}
		return nil
	}
	ok, err := uc.repos.Organizations.IsMember(ctx, orgId, actor.UserID)
	if err != nil {
		return mapRepoErr("authorize", err)
	}
	if !ok {
		return fmt.Errorf("%w: not a member of the organization", ErrForbidden)
	}
	return nil
}

// Start moves the mark from Confirmed to InProgress. Only a member of the
// organization the mark is assigned to may start it.
func (uc *Organizations) Start(ctx context.Context, actor models.Actor, markId int) (models.Mark, error) {
	const op = "usecase.Organizations.Start"

	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)

	var mark models.Mark
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		var err error
		mark, err = uc.lockAssignedMark(ctx, actor, markId)
		if err != nil {
			return err
		}
		if mark.MarkStatusID != models.ConfirmedStatus {
			return fmt.Errorf("%w: mark is not confirmed", ErrConflict)
		}
		return uc.setStatus(ctx, mark, models.InProgressStatus)
	})
	if err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}
	pending.Flush(ctx, uc.log, uc.events)

	uc.log.Info("mark started", slog.Int("mark_id", markId), slog.Int("user_id", actor.UserID))
	return uc.reload(ctx, op, markId)
}

// Resolve reports the mark as fixed: InProgress -> UnderReview. The report
// (comment + photos) is stored as a check of the service user on the
// in-progress stage, so it never counts as a vote of the review stage.
func (uc *Organizations) Resolve(ctx context.Context, actor models.Actor, markId int, comment string, photos []io.Reader) (models.Mark, error) {
	const op = "usecase.Organizations.Resolve"

	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)

	var mark models.Mark
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		var err error
		mark, err = uc.lockAssignedMark(ctx, actor, markId)
		if err != nil {
			return err
		}
		if mark.MarkStatusID != models.InProgressStatus {
			return fmt.Errorf("%w: mark is not in progress", ErrConflict)
		}

		historyItem, err := uc.repos.Marks.GetLastMarkStatusHistoryItem(ctx, markId)
		if err != nil {
			return err
		}
		checkId, err := uc.repos.Checks.AddCheck(ctx, models.Check{
			UserID:                  actor.UserID,
			MarkID:                  markId,
			MarkStatusId:            models.InProgressStatus,
			MarkStatusHistoryItemId: historyItem.ID,
			Result:                  true,
			Comment:                 comment,
		})
		if err != nil {
			return err
		}
		if err := uc.repos.Photos.AddPhotos(ctx, markId, int(checkId), photos); err != nil {
			return err
		}

		return uc.setStatus(ctx, mark, models.UnderReviewStatus)
	})
	if err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}
	pending.Flush(ctx, uc.log, uc.events)

	uc.log.Info("mark resolved", slog.Int("mark_id", markId), slog.Int("user_id", actor.UserID), slog.Int("photos", len(photos)))
	return uc.reload(ctx, op, markId)
}

// lockAssignedMark locks the mark and checks that the actor is a member of
// the organization it is assigned to (ErrForbidden otherwise; an
// unassigned mark yields ErrConflict).
func (uc *Organizations) lockAssignedMark(ctx context.Context, actor models.Actor, markId int) (models.Mark, error) {
	if err := uc.repos.Marks.LockMark(ctx, markId); err != nil {
		return models.Mark{}, err
	}
	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return models.Mark{}, err
	}
	if !mark.OrganizationID.Valid {
		return models.Mark{}, fmt.Errorf("%w: mark is not assigned to an organization", ErrConflict)
	}
	ok, err := uc.repos.Organizations.IsMember(ctx, int(mark.OrganizationID.Int64), actor.UserID)
	if err != nil {
		return models.Mark{}, err
	}
	if !ok {
		return models.Mark{}, fmt.Errorf("%w: not a member of the assigned organization", ErrForbidden)
	}
	return mark, nil
}

// setStatus writes the new status and queues mark.status_changed.
func (uc *Organizations) setStatus(ctx context.Context, mark models.Mark, newStatus models.MarkStatusType) error {
	if err := uc.repos.Marks.UpdateMarkStatus(ctx, mark.ID, newStatus); err != nil {
		return err
	}
	ev := events.NewMarkStatusChanged(mark.ID, mark.MarkStatusID, newStatus, mark.UserID)
	if !events.Collect(ctx, ev) {
		events.PublishEvent(ctx, uc.log, uc.events, ev)
	}
	return nil
}

func (uc *Organizations) reload(ctx context.Context, op string, markId int) (models.Mark, error) {
	mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
	if err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}
	return mark, nil
}

// Assign is the manual (re)assignment by a moderator: the mark must be
// confirmed or in progress (ErrConflict otherwise).
func (uc *Organizations) Assign(ctx context.Context, markId, orgId int) (models.Mark, error) {
	const op = "usecase.Organizations.Assign"

	if _, err := uc.repos.Organizations.GetOrganizationById(ctx, orgId); err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}

	var ev events.MarkAssigned
	err := uc.trManager.Do(ctx, func(ctx context.Context) error {
		if err := uc.repos.Marks.LockMark(ctx, markId); err != nil {
			return err
		}
		mark, err := uc.repos.Marks.GetMarkById(ctx, markId)
		if err != nil {
			return err
		}
		if mark.MarkStatusID != models.ConfirmedStatus && mark.MarkStatusID != models.InProgressStatus {
			return fmt.Errorf("%w: mark is neither confirmed nor in progress", ErrConflict)
		}
		ev, err = uc.assign(ctx, markId, orgId)
		return err
	})
	if err != nil {
		return models.Mark{}, mapRepoErr(op, err)
	}
	events.PublishEvent(ctx, uc.log, uc.events, ev)

	return uc.reload(ctx, op, markId)
}

// AssignConfirmed is the automatic assignment run by the status updater
// inside its transaction right after a mark became confirmed: the
// responsible organization is looked up by the mark's type and location;
// without one the mark stays unassigned. The event is queued on the
// context (published after the commit) when a Pending is present.
func (uc *Organizations) AssignConfirmed(ctx context.Context, mark models.Mark) error {
	const op = "usecase.Organizations.AssignConfirmed"

	org, err := uc.repos.Organizations.FindResponsibleOrganization(ctx, mark.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			uc.log.Debug("no responsible organization for mark", slog.String("op", op), slog.Int("mark_id", mark.ID))
			return nil
		}
		return mapRepoErr(op, err)
	}

	ev, err := uc.assign(ctx, mark.ID, org.ID)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if !events.Collect(ctx, ev) {
		events.PublishEvent(ctx, uc.log, uc.events, ev)
	}
	return nil
}

func (uc *Organizations) assign(ctx context.Context, markId, orgId int) (events.MarkAssigned, error) {
	dueAt, err := uc.repos.Organizations.AssignMark(ctx, markId, orgId)
	if err != nil {
		return events.MarkAssigned{}, err
	}
	uc.log.Info("mark assigned", slog.Int("mark_id", markId), slog.Int("organization_id", orgId), slog.Time("sla_due_at", dueAt))
	return events.NewMarkAssigned(markId, orgId, dueAt), nil
}
