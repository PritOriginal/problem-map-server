package usecase_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	testOrgID      = 10
	testMemberID   = 7
	testStrangerID = 8
)

type OrganizationsSuite struct {
	suite.Suite
	uc        *usecase.Organizations
	trManager *usecase.MockManager
	orgs      *usecase.MockOrganizationsRepository
	marks     *usecase.MockMarksRepository
	checks    *usecase.MockChecksRepository
	photos    *usecase.MockPhotosRepository
	users     *usecase.MockUsersRepository
	publisher *events.MockPublisher
}

func (suite *OrganizationsSuite) SetupTest() {
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.orgs = usecase.NewMockOrganizationsRepository(suite.T())
	suite.marks = usecase.NewMockMarksRepository(suite.T())
	suite.checks = usecase.NewMockChecksRepository(suite.T())
	suite.photos = usecase.NewMockPhotosRepository(suite.T())
	suite.users = usecase.NewMockUsersRepository(suite.T())
	suite.publisher = events.NewMockPublisher(suite.T())
	suite.uc = usecase.NewOrganizations(slogdiscard.NewDiscardLogger(), suite.trManager, usecase.OrganizationsRepositories{
		Organizations: suite.orgs,
		Marks:         suite.marks,
		Checks:        suite.checks,
		Photos:        suite.photos,
		Users:         suite.users,
	}).WithEvents(suite.publisher)
}

func TestOrganizations(t *testing.T) {
	suite.Run(t, new(OrganizationsSuite))
}

func assignedMark(status models.MarkStatusType) models.Mark {
	return models.Mark{ID: 5, UserID: 2, MarkStatusID: status, OrganizationID: null.IntFrom(testOrgID)}
}

func (suite *OrganizationsSuite) TestStart() {
	tests := []struct {
		name     string
		actor    models.Actor
		mark     models.Mark
		member   *bool
		wantErr  error
		wantSent bool
	}{
		{
			name: "OkMember", actor: models.Actor{UserID: testMemberID, Role: models.RoleService},
			mark: assignedMark(models.ConfirmedStatus), member: ptr(true), wantSent: true,
		},
		{
			// Admins act on any assigned mark without a membership lookup.
			name: "OkAdmin", actor: models.Actor{UserID: testStrangerID, Role: models.RoleAdmin},
			mark: assignedMark(models.ConfirmedStatus), wantSent: true,
		},
		{
			name: "ErrNotMember", actor: models.Actor{UserID: testStrangerID, Role: models.RoleService},
			mark: assignedMark(models.ConfirmedStatus), member: ptr(false), wantErr: usecase.ErrForbidden,
		},
		{
			name: "ErrNotAssigned", actor: models.Actor{UserID: testMemberID, Role: models.RoleService},
			mark: models.Mark{ID: 5, MarkStatusID: models.ConfirmedStatus}, wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrWrongStatus", actor: models.Actor{UserID: testMemberID, Role: models.RoleService},
			mark: assignedMark(models.InProgressStatus), member: ptr(true), wantErr: usecase.ErrConflict,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
			suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
			suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(tt.mark, nil)
			if tt.member != nil {
				suite.orgs.On("IsMember", mock.Anything, testOrgID, tt.actor.UserID).Once().Return(*tt.member, nil)
			}
			if tt.wantSent {
				suite.marks.On("UpdateMarkStatus", mock.Anything, 5, models.InProgressStatus).Once().Return(nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkStatusChanged, mock.MatchedBy(func(ev events.MarkStatusChanged) bool {
					return ev.MarkID == 5 && ev.OldStatus == models.ConfirmedStatus && ev.NewStatus == models.InProgressStatus
				})).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(assignedMark(models.InProgressStatus), nil)
			}

			got, err := suite.uc.Start(context.Background(), tt.actor, 5)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(models.InProgressStatus, got.MarkStatusID)
		})
	}
}

func (suite *OrganizationsSuite) TestResolve() {
	tests := []struct {
		name    string
		actor   models.Actor
		mark    models.Mark
		member  *bool
		wantErr error
	}{
		{
			name: "Ok", actor: models.Actor{UserID: testMemberID, Role: models.RoleService},
			mark: assignedMark(models.InProgressStatus), member: ptr(true),
		},
		{
			name: "ErrNotMember", actor: models.Actor{UserID: testStrangerID, Role: models.RoleService},
			mark: assignedMark(models.InProgressStatus), member: ptr(false), wantErr: usecase.ErrForbidden,
		},
		{
			name: "ErrNotInProgress", actor: models.Actor{UserID: testMemberID, Role: models.RoleService},
			mark: assignedMark(models.ConfirmedStatus), member: ptr(true), wantErr: usecase.ErrConflict,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
			suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
			suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(tt.mark, nil)
			if tt.member != nil {
				suite.orgs.On("IsMember", mock.Anything, testOrgID, tt.actor.UserID).Once().Return(*tt.member, nil)
			}
			if tt.wantErr == nil {
				suite.marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).Once().
					Return(models.MarkStatusHistoryItem{ID: 42, NewMarkStatusID: models.InProgressStatus}, nil)
				suite.checks.On("AddCheck", mock.Anything, mock.MatchedBy(func(c models.Check) bool {
					return c.UserID == testMemberID && c.MarkID == 5 && c.Result && c.Comment == "done" &&
						c.MarkStatusHistoryItemId == 42 && c.MarkStatusId == models.InProgressStatus
				})).Once().Return(int64(9), nil)
				suite.photos.On("AddPhotos", mock.Anything, 5, 9, mock.Anything).Once().Return(nil)
				suite.marks.On("UpdateMarkStatus", mock.Anything, 5, models.UnderReviewStatus).Once().Return(nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkStatusChanged, mock.MatchedBy(func(ev events.MarkStatusChanged) bool {
					return ev.OldStatus == models.InProgressStatus && ev.NewStatus == models.UnderReviewStatus
				})).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(assignedMark(models.UnderReviewStatus), nil)
			}

			got, err := suite.uc.Resolve(context.Background(), tt.actor, 5, "done", []io.Reader{})
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(models.UnderReviewStatus, got.MarkStatusID)
		})
	}
}

func (suite *OrganizationsSuite) TestAssignConfirmed() {
	dueAt := time.Now().Add(72 * time.Hour)

	tests := []struct {
		name        string
		findOrg     method[models.Organization]
		wantAssign  bool
		wantErr     bool
		withPending bool
	}{
		{name: "OkPublished", findOrg: method[models.Organization]{data: models.Organization{ID: testOrgID}}, wantAssign: true},
		{name: "OkQueued", findOrg: method[models.Organization]{data: models.Organization{ID: testOrgID}}, wantAssign: true, withPending: true},
		{name: "NoOrganization", findOrg: method[models.Organization]{err: repository.ErrNotFound}},
		{name: "ErrRepo", findOrg: method[models.Organization]{err: errRepo}, wantErr: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			suite.orgs.On("FindResponsibleOrganization", mock.Anything, 5).Once().Return(tt.findOrg.data, tt.findOrg.err)
			if tt.wantAssign {
				suite.orgs.On("AssignMark", mock.Anything, 5, testOrgID).Once().Return(dueAt, nil)
			}
			ctx := context.Background()
			var pending events.Pending
			if tt.withPending {
				ctx = events.WithPending(ctx, &pending)
			} else if tt.wantAssign {
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkAssigned, mock.MatchedBy(func(ev events.MarkAssigned) bool {
					return ev.MarkID == 5 && ev.OrganizationID == testOrgID && ev.SLADueAt.Equal(dueAt)
				})).Once().Return(nil)
			}

			err := suite.uc.AssignConfirmed(ctx, models.Mark{ID: 5, MarkStatusID: models.ConfirmedStatus})
			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.NoError(err)
			if tt.withPending {
				suite.Len(pending.Events(), 1)
			}
		})
	}
}

func (suite *OrganizationsSuite) TestAssign() {
	dueAt := time.Now().Add(time.Hour)

	tests := []struct {
		name    string
		getOrg  method[models.Organization]
		mark    models.Mark
		wantErr error
	}{
		{name: "Ok", getOrg: method[models.Organization]{data: models.Organization{ID: testOrgID}}, mark: models.Mark{ID: 5, MarkStatusID: models.ConfirmedStatus}},
		{name: "OkReassign", getOrg: method[models.Organization]{data: models.Organization{ID: testOrgID}}, mark: assignedMark(models.InProgressStatus)},
		{name: "ErrOrgNotFound", getOrg: method[models.Organization]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "ErrStatus", getOrg: method[models.Organization]{data: models.Organization{ID: testOrgID}}, mark: models.Mark{ID: 5, MarkStatusID: models.UnconfirmedStatus}, wantErr: usecase.ErrConflict},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			suite.orgs.On("GetOrganizationById", mock.Anything, testOrgID).Once().Return(tt.getOrg.data, tt.getOrg.err)
			if tt.getOrg.err == nil {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(tt.mark, nil)
			}
			if tt.wantErr == nil {
				suite.orgs.On("AssignMark", mock.Anything, 5, testOrgID).Once().Return(dueAt, nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkAssigned, mock.Anything).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(assignedMark(tt.mark.MarkStatusID), nil)
			}

			got, err := suite.uc.Assign(context.Background(), 5, testOrgID)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(int64(testOrgID), got.OrganizationID.Int64)
		})
	}
}

func (suite *OrganizationsSuite) TestAddMember() {
	tests := []struct {
		name    string
		user    method[models.User]
		addErr  error
		wantErr error
	}{
		{name: "Ok", user: method[models.User]{data: models.User{Id: testMemberID, Role: models.RoleUser}}},
		{name: "ErrUserNotFound", user: method[models.User]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "ErrModerator", user: method[models.User]{data: models.User{Id: testMemberID, Role: models.RoleModerator}}, wantErr: usecase.ErrConflict},
		{name: "ErrAlreadyMember", user: method[models.User]{data: models.User{Id: testMemberID, Role: models.RoleUser}}, addErr: repository.ErrExists, wantErr: usecase.ErrConflict},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			suite.orgs.On("GetOrganizationById", mock.Anything, testOrgID).Once().Return(models.Organization{ID: testOrgID}, nil)
			suite.users.On("GetUserById", mock.Anything, testMemberID).Once().Return(tt.user.data, tt.user.err)
			if tt.user.err == nil && tt.user.data.Role == models.RoleUser {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
				suite.orgs.On("AddMember", mock.Anything, testOrgID, testMemberID).Once().Return(tt.addErr)
				if tt.addErr == nil {
					suite.users.On("UpdateRole", mock.Anything, testMemberID, models.RoleService).Once().Return(nil)
				}
			}

			err := suite.uc.AddMember(context.Background(), testOrgID, testMemberID)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *OrganizationsSuite) TestListMarks() {
	tests := []struct {
		name    string
		actor   models.Actor
		filters models.GetOrganizationMarksFilters
		member  *bool
		wantErr error
	}{
		{name: "OkMember", actor: models.Actor{UserID: testMemberID, Role: models.RoleService}, member: ptr(true), filters: models.GetOrganizationMarksFilters{Overdue: true}},
		{name: "OkAdmin", actor: models.Actor{UserID: 1, Role: models.RoleAdmin}},
		{name: "ErrStranger", actor: models.Actor{UserID: testStrangerID, Role: models.RoleService}, member: ptr(false), wantErr: usecase.ErrForbidden},
		{name: "ErrPagination", actor: models.Actor{UserID: 1, Role: models.RoleAdmin}, filters: models.GetOrganizationMarksFilters{Pagination: models.Pagination{Limit: -1}}, wantErr: usecase.ErrInvalidArgument},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			if tt.member != nil {
				suite.orgs.On("IsMember", mock.Anything, testOrgID, tt.actor.UserID).Once().Return(*tt.member, nil)
			} else if tt.wantErr == nil {
				suite.orgs.On("GetOrganizationById", mock.Anything, testOrgID).Once().Return(models.Organization{ID: testOrgID}, nil)
			}
			if tt.wantErr == nil {
				suite.orgs.On("GetOrganizationMarks", mock.Anything, testOrgID, tt.filters).Once().
					Return(models.Page[models.Mark]{Items: []models.Mark{assignedMark(models.ConfirmedStatus)}, Total: 1}, nil)
			}

			page, err := suite.uc.ListMarks(context.Background(), tt.actor, testOrgID, tt.filters)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(1, page.Total)
		})
	}
}

func ptr[T any](v T) *T { return &v }

// TestUpdaterAssignsConfirmedMark checks the hook: a mark that becomes
// confirmed is handed to the assigner inside the transaction, and the
// assignment event is published after the commit.
func (suite *OrganizationsSuite) TestUpdaterAssignsConfirmedMark() {
	dueAt := time.Now().Add(time.Hour)
	assigner := usecase.NewMockMarkAssigner(suite.T())
	updater := usecase.NewUpdater(slogdiscard.NewDiscardLogger(), ratingCfg, suite.trManager, usecase.UpdaterRepositories{
		Marks:  suite.marks,
		Checks: suite.checks,
		Users:  suite.users,
	}).WithEvents(suite.publisher).WithAssigner(assigner)

	mark := models.Mark{ID: 5, UserID: 2, MarkStatusID: models.UnconfirmedStatus}
	suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
	suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
	suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(mark, nil)
	suite.marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).Once().Return(models.MarkStatusHistoryItem{ID: 1}, nil)
	suite.checks.On("GetChecksByMarkHistoryId", mock.Anything, 1).Once().Return([]models.Check{}, nil)
	suite.marks.On("UpdateMarkStatus", mock.Anything, 5, models.ConfirmedStatus).Once().Return(nil)
	assigner.On("AssignConfirmed", mock.Anything, mark).Once().Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		suite.True(events.Collect(ctx, events.NewMarkAssigned(5, testOrgID, dueAt)), "event must be queued until the commit")
	}).Return(nil)
	suite.users.On("AddRatingEvent", mock.Anything, mock.Anything).Once().Return(int64(1), nil)

	var published []string
	suite.publisher.On("Publish", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		published = append(published, args.String(1))
	}).Return(nil)

	got, err := updater.Confirm(context.Background(), 5)
	suite.Require().NoError(err)
	suite.Equal(models.ConfirmedStatus, got)
	suite.Equal([]string{events.SubjectMarkAssigned, events.SubjectMarkStatusChanged}, published)

	// A failing assignment rolls the decision back.
	suite.SetupTest()
	assigner = usecase.NewMockMarkAssigner(suite.T())
	updater = usecase.NewUpdater(slogdiscard.NewDiscardLogger(), ratingCfg, suite.trManager, usecase.UpdaterRepositories{
		Marks: suite.marks, Checks: suite.checks, Users: suite.users,
	}).WithAssigner(assigner)
	suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runInTx)
	suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
	suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(mark, nil)
	suite.marks.On("GetLastMarkStatusHistoryItem", mock.Anything, 5).Once().Return(models.MarkStatusHistoryItem{ID: 1}, nil)
	suite.checks.On("GetChecksByMarkHistoryId", mock.Anything, 1).Once().Return([]models.Check{}, nil)
	suite.marks.On("UpdateMarkStatus", mock.Anything, 5, models.ConfirmedStatus).Once().Return(nil)
	assigner.On("AssignConfirmed", mock.Anything, mark).Once().Return(errors.New("boom"))

	_, err = updater.Confirm(context.Background(), 5)
	suite.Error(err)
}
