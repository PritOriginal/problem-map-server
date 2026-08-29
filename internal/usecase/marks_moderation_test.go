package usecase_test

import (
	"context"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// MarksModerationSuite covers the moderation part of Marks: visibility of
// hidden marks, hiding by a moderator and the merge of duplicates.
type MarksModerationSuite struct {
	suite.Suite
	uc        *usecase.Marks
	trManager *usecase.MockManager
	marks     *usecase.MockMarksRepository
	tasks     *usecase.MockMarkTasksRepository
	reports   *usecase.MockMarkReportsRepository
	publisher *events.MockPublisher
}

func (suite *MarksModerationSuite) SetupTest() {
	suite.trManager = usecase.NewMockManager(suite.T())
	suite.marks = usecase.NewMockMarksRepository(suite.T())
	suite.tasks = usecase.NewMockMarkTasksRepository(suite.T())
	suite.reports = usecase.NewMockMarkReportsRepository(suite.T())
	suite.publisher = events.NewMockPublisher(suite.T())
	suite.uc = usecase.NewMarks(slogdiscard.NewDiscardLogger(), config.MarksConfig{DedupRadiusM: 50}, suite.trManager, usecase.MarksRepositories{
		Marks:   suite.marks,
		Checks:  usecase.NewMockChecksRepository(suite.T()),
		Photos:  usecase.NewMockPhotosRepository(suite.T()),
		Tasks:   suite.tasks,
		Reports: suite.reports,
	}).WithEvents(suite.publisher)
}

func TestMarksModeration(t *testing.T) {
	suite.Run(t, new(MarksModerationSuite))
}

var (
	modActor  = models.Actor{UserID: 2, Role: models.RoleModerator}
	userActor = models.Actor{UserID: 7, Role: models.RoleUser}
)

func (suite *MarksModerationSuite) TestGetMarkById_Visibility() {
	hidden := models.Mark{ID: 5, UserID: 3, Hidden: true}
	visible := models.Mark{ID: 6, UserID: 3}

	tests := []struct {
		name    string
		mark    models.Mark
		ctx     context.Context
		wantErr error
	}{
		{name: "VisibleAnonymous", mark: visible, ctx: context.Background()},
		{name: "HiddenAnonymous", mark: hidden, ctx: context.Background(), wantErr: usecase.ErrNotFound},
		{name: "HiddenStranger", mark: hidden, ctx: models.ContextWithActor(context.Background(), userActor), wantErr: usecase.ErrNotFound},
		{name: "HiddenAuthor", mark: hidden, ctx: models.ContextWithViewer(context.Background(), 3)},
		{name: "HiddenModerator", mark: hidden, ctx: models.ContextWithActor(context.Background(), modActor)},
		{name: "HiddenAdmin", mark: hidden, ctx: models.ContextWithActor(context.Background(), models.Actor{UserID: 9, Role: models.RoleAdmin})},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.marks.On("GetMarkById", mock.Anything, tt.mark.ID).Once().Return(tt.mark, nil)

			got, err := suite.uc.GetMarkById(tt.ctx, tt.mark.ID)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				suite.Equal(models.Mark{}, got, "a hidden mark must not leak")
				return
			}
			suite.NoError(err)
			suite.Equal(tt.mark, got)
		})
	}
}

func (suite *MarksModerationSuite) TestSetHidden() {
	tests := []struct {
		name    string
		actor   models.Actor
		hidden  bool
		setup   func()
		want    models.Mark
		wantErr error
	}{
		{name: "ErrUserForbidden", actor: userActor, hidden: true, wantErr: usecase.ErrForbidden},
		{
			name: "ErrNotFound", actor: modActor, hidden: true,
			setup: func() {
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{}, repository.ErrNotFound)
			},
			wantErr: usecase.ErrNotFound,
		},
		{
			name: "OkHidePublishes", actor: modActor, hidden: true,
			setup: func() {
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
				suite.marks.On("SetMarkHidden", mock.Anything, 5, true).Once().Return(nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkHidden, mock.MatchedBy(func(ev events.MarkHidden) bool {
					return ev.EventID != "" && ev.MarkID == 5 && ev.AuthorID == 3 && ev.ModeratorID == 2 && ev.ReportsCount == 0
				})).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3, Hidden: true}, nil)
			},
			want: models.Mark{ID: 5, UserID: 3, Hidden: true},
		},
		{
			name: "OkUnhideDoesNotPublish", actor: models.Actor{UserID: 1, Role: models.RoleAdmin}, hidden: false,
			setup: func() {
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3, Hidden: true}, nil)
				suite.marks.On("SetMarkHidden", mock.Anything, 5, false).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
			},
			want: models.Mark{ID: 5, UserID: 3},
		},
		{
			name: "OkAlreadyHiddenIsNoop", actor: modActor, hidden: true,
			setup: func() {
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3, Hidden: true}, nil)
			},
			want: models.Mark{ID: 5, UserID: 3, Hidden: true},
		},
		{
			name: "ErrUpdate", actor: modActor, hidden: true,
			setup: func() {
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3}, nil)
				suite.marks.On("SetMarkHidden", mock.Anything, 5, true).Once().Return(errRepo)
			},
			wantErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			got, err := suite.uc.SetHidden(context.Background(), tt.actor, 5, tt.hidden)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(tt.want, got)
			suite.publisher.AssertExpectations(suite.T())
		})
	}
}

func (suite *MarksModerationSuite) TestMergeInto() {
	source := models.Mark{ID: 5, UserID: 3, MarkStatusID: models.ConfirmedStatus}
	target := models.Mark{ID: 2, UserID: 4, MarkStatusID: models.UnconfirmedStatus}

	// lockBoth expects the row locks in id order regardless of the direction
	// of the merge.
	lockBoth := func() {
		suite.marks.On("LockMark", mock.Anything, 2).Once().Return(nil)
		suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
	}

	tests := []struct {
		name     string
		actor    models.Actor
		markId   int
		targetId int
		setup    func()
		wantErr  error
	}{
		{name: "ErrUserForbidden", actor: userActor, markId: 5, targetId: 2, wantErr: usecase.ErrForbidden},
		{name: "ErrSameMark", actor: modActor, markId: 5, targetId: 5, wantErr: usecase.ErrInvalidArgument},
		{
			name: "ErrSourceNotFound", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				suite.marks.On("LockMark", mock.Anything, 2).Once().Return(nil)
				suite.marks.On("LockMark", mock.Anything, 5).Once().Return(repository.ErrNotFound)
			},
			wantErr: usecase.ErrNotFound,
		},
		{
			name: "ErrTargetNotFound", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				suite.marks.On("LockMark", mock.Anything, 2).Once().Return(repository.ErrNotFound)
			},
			wantErr: usecase.ErrNotFound,
		},
		{
			name: "ErrSourceNotActive", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				lockBoth()
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3, MarkStatusID: models.DuplicateStatus}, nil)
				suite.marks.On("GetMarkById", mock.Anything, 2).Once().Return(target, nil)
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrTargetNotActive", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				lockBoth()
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(source, nil)
				suite.marks.On("GetMarkById", mock.Anything, 2).Once().Return(models.Mark{ID: 2, UserID: 4, MarkStatusID: models.ClosedStatus}, nil)
			},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrTargetIsDuplicate", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				lockBoth()
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(source, nil)
				suite.marks.On("GetMarkById", mock.Anything, 2).Once().
					Return(models.Mark{ID: 2, UserID: 4, MarkStatusID: models.DuplicateStatus, MergedIntoID: null.IntFrom(9)}, nil)
			},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name: "OkMovesEverythingAndPublishes", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				lockBoth()
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(source, nil)
				suite.marks.On("GetMarkById", mock.Anything, 2).Once().Return(target, nil)
				suite.marks.On("GetFollowerIDs", mock.Anything, 5).Once().Return([]int{3, 8}, nil)
				suite.marks.On("MoveFollowers", mock.Anything, 5, 2).Once().Return(nil)
				suite.tasks.On("MoveOpenTasks", mock.Anything, 5, 2).Once().Return(nil)
				suite.reports.On("MoveMarkReports", mock.Anything, 5, 2).Once().Return(nil)
				suite.marks.On("MergeMark", mock.Anything, 5, 2).Once().Return(nil)
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkMerged, mock.MatchedBy(func(ev events.MarkMerged) bool {
					return ev.EventID != "" && ev.MarkID == 5 && ev.TargetMarkID == 2 && ev.AuthorID == 3 && ev.ModeratorID == 2 &&
						len(ev.FollowerIDs) == 2 && ev.FollowerIDs[0] == 3 && ev.FollowerIDs[1] == 8
				})).Once().Return(nil)
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: 3, MarkStatusID: models.DuplicateStatus}, nil)
			},
		},
		{
			name: "RolledBackMergeIsNotPublished", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(errRepo))
				lockBoth()
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(source, nil)
				suite.marks.On("GetMarkById", mock.Anything, 2).Once().Return(target, nil)
				suite.marks.On("GetFollowerIDs", mock.Anything, 5).Once().Return([]int{}, nil)
				suite.marks.On("MoveFollowers", mock.Anything, 5, 2).Once().Return(nil)
				suite.tasks.On("MoveOpenTasks", mock.Anything, 5, 2).Once().Return(nil)
				suite.reports.On("MoveMarkReports", mock.Anything, 5, 2).Once().Return(nil)
				suite.marks.On("MergeMark", mock.Anything, 5, 2).Once().Return(nil)
			},
			wantErr: errRepo,
		},
		{
			name: "ErrMoveTasksStopsTheMerge", actor: modActor, markId: 5, targetId: 2,
			setup: func() {
				suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
				lockBoth()
				suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(source, nil)
				suite.marks.On("GetMarkById", mock.Anything, 2).Once().Return(target, nil)
				suite.marks.On("GetFollowerIDs", mock.Anything, 5).Once().Return([]int{}, nil)
				suite.marks.On("MoveFollowers", mock.Anything, 5, 2).Once().Return(nil)
				suite.tasks.On("MoveOpenTasks", mock.Anything, 5, 2).Once().Return(errRepo)
			},
			wantErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			got, err := suite.uc.MergeInto(context.Background(), tt.actor, tt.markId, tt.targetId)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.Require().NoError(err)
				suite.Equal(models.DuplicateStatus, got.MarkStatusID)
			}
			suite.marks.AssertExpectations(suite.T())
			suite.tasks.AssertExpectations(suite.T())
			suite.reports.AssertExpectations(suite.T())
			suite.publisher.AssertExpectations(suite.T())
		})
	}
}

// TestMergeInto_WithoutOptionalRepos: a Marks built without the tasks and
// reports repositories still merges (followers and status only).
func (suite *MarksModerationSuite) TestMergeInto_WithoutOptionalRepos() {
	uc := usecase.NewMarks(slogdiscard.NewDiscardLogger(), config.MarksConfig{}, suite.trManager, usecase.MarksRepositories{
		Marks: suite.marks,
	})

	suite.trManager.On("Do", mock.Anything, mock.Anything).Once().Return(runTx(nil))
	suite.marks.On("LockMark", mock.Anything, 2).Once().Return(nil)
	suite.marks.On("LockMark", mock.Anything, 5).Once().Return(nil)
	suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, MarkStatusID: models.UnconfirmedStatus}, nil)
	suite.marks.On("GetMarkById", mock.Anything, 2).Once().Return(models.Mark{ID: 2, MarkStatusID: models.InProgressStatus}, nil)
	suite.marks.On("GetFollowerIDs", mock.Anything, 5).Once().Return(nil, nil)
	suite.marks.On("MoveFollowers", mock.Anything, 5, 2).Once().Return(nil)
	suite.marks.On("MergeMark", mock.Anything, 5, 2).Once().Return(nil)
	suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, MarkStatusID: models.DuplicateStatus}, nil)

	got, err := uc.MergeInto(context.Background(), modActor, 5, 2)

	suite.Require().NoError(err)
	suite.Equal(models.DuplicateStatus, got.MarkStatusID)
}

// TestGetMarkStatusHistoryByMarkId_Hidden: the history of a hidden mark is
// ErrNotFound for a stranger and readable by the author.
func (suite *MarksModerationSuite) TestGetMarkStatusHistoryByMarkId_Hidden() {
	hidden := models.Mark{ID: 5, UserID: 3, Hidden: true}

	suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(hidden, nil)
	_, err := suite.uc.GetMarkStatusHistoryByMarkId(models.ContextWithActor(context.Background(), userActor), 5, false)
	suite.ErrorIs(err, usecase.ErrNotFound)
	suite.marks.AssertNotCalled(suite.T(), "GetMarkStatusHistoryByMarkId", mock.Anything, mock.Anything)

	suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(hidden, nil)
	suite.marks.On("GetMarkStatusHistoryByMarkId", mock.Anything, 5).Once().Return([]models.MarkStatusHistoryItem{{ID: 1}}, nil)
	items, err := suite.uc.GetMarkStatusHistoryByMarkId(models.ContextWithViewer(context.Background(), 3), 5, false)
	suite.NoError(err)
	suite.Len(items, 1)
}
