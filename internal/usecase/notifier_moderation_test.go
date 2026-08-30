package usecase_test

import (
	"context"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// NotifierModerationSuite covers the notifications of the moderation
// events (mark.hidden, mark.merged).
type NotifierModerationSuite struct {
	suite.Suite
	uc            *usecase.Notifier
	notifications *usecase.MockNotificationCreator
	marks         *usecase.MockNotifierMarksRepository
	users         *usecase.MockNotifierUsersRepository
}

func (suite *NotifierModerationSuite) SetupTest() {
	suite.notifications = usecase.NewMockNotificationCreator(suite.T())
	suite.marks = usecase.NewMockNotifierMarksRepository(suite.T())
	suite.users = usecase.NewMockNotifierUsersRepository(suite.T())
	suite.uc = usecase.NewNotifier(slogdiscard.NewDiscardLogger(), suite.notifications, usecase.NotifierRepositories{
		Marks: suite.marks,
		Users: suite.users,
	})
}

func TestNotifierModeration(t *testing.T) {
	suite.Run(t, new(NotifierModerationSuite))
}

// notified records the addressees of every created notification.
func (suite *NotifierModerationSuite) notified(ntype models.NotificationType, eventID string, markID int) *[]int {
	got := &[]int{}
	suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
		return n.Type == ntype && n.EventID == eventID && n.MarkID.ValueOrZero() == int64(markID) && n.Title != "" && n.Body != ""
	})).Run(func(args mock.Arguments) {
		*got = append(*got, args.Get(1).(models.Notification).UserID)
	}).Return(int64(1), true, nil).Maybe()
	return got
}

func (suite *NotifierModerationSuite) TestHandleMarkHidden() {
	tests := []struct {
		name     string
		ev       events.MarkHidden
		getMark  *method[models.Mark]
		roles    method[[]int]
		wantSent []int
		wantErr  error
	}{
		{
			name: "AuthorAndModerators",
			ev:   events.MarkHidden{Header: events.Header{EventID: "h1"}, MarkID: 5, AuthorID: 3, ReportsCount: 5},
			// The author is a moderator too: notified once.
			roles:    method[[]int]{data: []int{2, 3, 9}},
			wantSent: []int{3, 2, 9},
		},
		{
			name:     "ByModeratorNoModeratorsConfigured",
			ev:       events.MarkHidden{Header: events.Header{EventID: "h2"}, MarkID: 5, AuthorID: 3, ModeratorID: 2},
			roles:    method[[]int]{data: []int{}},
			wantSent: []int{3},
		},
		{
			name:     "AuthorFromMark",
			ev:       events.MarkHidden{Header: events.Header{EventID: "h3"}, MarkID: 5},
			getMark:  &method[models.Mark]{data: models.Mark{ID: 5, UserID: 4}},
			roles:    method[[]int]{data: []int{2}},
			wantSent: []int{4, 2},
		},
		{
			name:    "ErrMarkNotFound",
			ev:      events.MarkHidden{Header: events.Header{EventID: "h4"}, MarkID: 5},
			getMark: &method[models.Mark]{err: repository.ErrNotFound},
			wantErr: usecase.ErrNotFound,
		},
		{
			name:     "ErrRoles",
			ev:       events.MarkHidden{Header: events.Header{EventID: "h5"}, MarkID: 5, AuthorID: 3},
			roles:    method[[]int]{err: errRepo},
			wantSent: []int{3},
			wantErr:  errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getMark != nil {
				suite.marks.On("GetMarkById", mock.Anything, tt.ev.MarkID).Once().Return(tt.getMark.data, tt.getMark.err)
			}
			if tt.getMark == nil || tt.getMark.err == nil {
				suite.users.On("GetUserIDsByRole", mock.Anything, []models.Role{models.RoleModerator, models.RoleAdmin}).Once().Return(tt.roles.data, tt.roles.err)
			}
			sent := suite.notified(models.NotificationMarkHidden, tt.ev.EventID, tt.ev.MarkID)

			err := suite.uc.HandleMarkHidden(context.Background(), tt.ev)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
			}
			suite.Equal(append([]int{}, tt.wantSent...), *sent)
		})
	}
}

func (suite *NotifierModerationSuite) TestHandleMarkHidden_UsersNotConfigured() {
	uc := usecase.NewNotifier(slogdiscard.NewDiscardLogger(), suite.notifications, usecase.NotifierRepositories{Marks: suite.marks})

	err := uc.HandleMarkHidden(context.Background(), events.MarkHidden{Header: events.Header{EventID: "h"}, MarkID: 5, AuthorID: 3})

	suite.Error(err)
}

func (suite *NotifierModerationSuite) TestHandleMarkMerged() {
	tests := []struct {
		name     string
		ev       events.MarkMerged
		getMark  *method[models.Mark]
		wantSent []int
		wantErr  error
	}{
		{
			name: "AuthorAndFollowers",
			// The author follows their own mark: notified once.
			ev:       events.MarkMerged{Header: events.Header{EventID: "m1"}, MarkID: 5, TargetMarkID: 2, AuthorID: 3, FollowerIDs: []int{3, 8, 9}},
			wantSent: []int{3, 8, 9},
		},
		{
			name:     "NoFollowers",
			ev:       events.MarkMerged{Header: events.Header{EventID: "m2"}, MarkID: 5, TargetMarkID: 2, AuthorID: 3},
			wantSent: []int{3},
		},
		{
			name:     "AuthorFromMark",
			ev:       events.MarkMerged{Header: events.Header{EventID: "m3"}, MarkID: 5, TargetMarkID: 2, FollowerIDs: []int{8}},
			getMark:  &method[models.Mark]{data: models.Mark{ID: 5, UserID: 4}},
			wantSent: []int{4, 8},
		},
		{
			name:    "ErrMarkNotFound",
			ev:      events.MarkMerged{Header: events.Header{EventID: "m4"}, MarkID: 5, TargetMarkID: 2},
			getMark: &method[models.Mark]{err: repository.ErrNotFound},
			wantErr: usecase.ErrNotFound,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getMark != nil {
				suite.marks.On("GetMarkById", mock.Anything, tt.ev.MarkID).Once().Return(tt.getMark.data, tt.getMark.err)
			}
			// The notification links to the target mark: the merged one is gone.
			sent := suite.notified(models.NotificationMarkMerged, tt.ev.EventID, tt.ev.TargetMarkID)

			err := suite.uc.HandleMarkMerged(context.Background(), tt.ev)

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
			}
			suite.Equal(append([]int{}, tt.wantSent...), *sent)
		})
	}
}

func (suite *NotifierModerationSuite) TestHandleMarkMerged_CreateError() {
	suite.notifications.On("Create", mock.Anything, mock.Anything).Once().Return(int64(0), false, errRepo)

	err := suite.uc.HandleMarkMerged(context.Background(), events.MarkMerged{Header: events.Header{EventID: "m"}, MarkID: 5, TargetMarkID: 2, AuthorID: 3, FollowerIDs: []int{8}})

	suite.ErrorIs(err, errRepo)
}
