package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type NotifierSuite struct {
	suite.Suite
	uc            *usecase.Notifier
	notifications *usecase.MockNotificationCreator
	marks         *usecase.MockNotifierMarksRepository
}

func (suite *NotifierSuite) SetupTest() {
	suite.notifications = usecase.NewMockNotificationCreator(suite.T())
	suite.marks = usecase.NewMockNotifierMarksRepository(suite.T())
	suite.uc = usecase.NewNotifier(slogdiscard.NewDiscardLogger(), suite.notifications, usecase.NotifierRepositories{
		Marks: suite.marks,
	})
}

func TestNotifier(t *testing.T) {
	suite.Run(t, new(NotifierSuite))
}

func (suite *NotifierSuite) TestHandleMarkStatusChanged() {
	tests := []struct {
		name       string
		ev         events.MarkStatusChanged
		getMark    *method[models.Mark]
		createErr  error
		wantUserID int
		wantErr    bool
	}{
		{
			name:       "OkAuthorFromEvent",
			ev:         events.MarkStatusChanged{Header: events.Header{EventID: "e1"}, MarkID: 5, OldStatus: models.UnconfirmedStatus, NewStatus: models.ConfirmedStatus, AuthorID: 3},
			wantUserID: 3,
		},
		{
			name:       "OkAuthorFromMark",
			ev:         events.MarkStatusChanged{Header: events.Header{EventID: "e2"}, MarkID: 5, OldStatus: models.UnconfirmedStatus, NewStatus: models.MarkStatusType(99)},
			getMark:    &method[models.Mark]{data: models.Mark{ID: 5, UserID: 4}},
			wantUserID: 4,
		},
		{
			name:    "ErrMarkNotFound",
			ev:      events.MarkStatusChanged{Header: events.Header{EventID: "e3"}, MarkID: 5},
			getMark: &method[models.Mark]{err: repository.ErrNotFound},
			wantErr: true,
		},
		{
			name:       "ErrCreate",
			ev:         events.MarkStatusChanged{Header: events.Header{EventID: "e4"}, MarkID: 5, AuthorID: 3},
			createErr:  errRepo,
			wantUserID: 3,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getMark != nil {
				suite.marks.On("GetMarkById", mock.Anything, tt.ev.MarkID).Once().Return(tt.getMark.data, tt.getMark.err)
			}
			if tt.getMark == nil || tt.getMark.err == nil {
				suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
					return n.UserID == tt.wantUserID &&
						n.EventID == tt.ev.EventID &&
						n.Type == models.NotificationMarkStatusChanged &&
						n.MarkID.ValueOrZero() == int64(tt.ev.MarkID) &&
						!n.TaskID.Valid &&
						n.Title != "" && n.Body != ""
				})).Once().Return(int64(1), true, tt.createErr)
			}

			err := suite.uc.HandleMarkStatusChanged(context.Background(), tt.ev)
			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *NotifierSuite) TestHandleTaskAssigned() {
	due := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	tests := []struct {
		name      string
		ev        events.TaskAssigned
		createErr error
		wantBody  string
	}{
		{
			name:     "OkWithDueAt",
			ev:       events.TaskAssigned{Header: events.Header{EventID: "e1"}, TaskID: 9, UserID: 2, MarkID: 5, DueAt: &due},
			wantBody: "Вам назначена проверка метки #5 до 02.01.2026 15:04",
		},
		{
			name:     "OkWithoutDueAt",
			ev:       events.TaskAssigned{Header: events.Header{EventID: "e2"}, TaskID: 9, UserID: 2, MarkID: 5},
			wantBody: "Вам назначена проверка метки #5",
		},
		{
			name:      "ErrCreate",
			ev:        events.TaskAssigned{Header: events.Header{EventID: "e3"}, TaskID: 9, UserID: 2, MarkID: 5},
			createErr: errRepo,
			wantBody:  "Вам назначена проверка метки #5",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
				return n.UserID == tt.ev.UserID &&
					n.EventID == tt.ev.EventID &&
					n.Type == models.NotificationTaskAssigned &&
					n.MarkID.ValueOrZero() == int64(tt.ev.MarkID) &&
					n.TaskID.ValueOrZero() == int64(tt.ev.TaskID) &&
					n.Body == tt.wantBody
			})).Once().Return(int64(1), true, tt.createErr)

			err := suite.uc.HandleTaskAssigned(context.Background(), tt.ev)
			if tt.createErr != nil {
				suite.ErrorIs(err, tt.createErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *NotifierSuite) TestHandleCheckAdded() {
	tests := []struct {
		name       string
		ev         events.CheckAdded
		getMark    method[models.Mark]
		wantCreate bool
		createErr  error
		wantErr    bool
	}{
		{
			name:       "OkNotifiesAuthor",
			ev:         events.CheckAdded{Header: events.Header{EventID: "e1"}, CheckID: 1, MarkID: 5, UserID: 2},
			getMark:    method[models.Mark]{data: models.Mark{ID: 5, UserID: 3}},
			wantCreate: true,
		},
		{
			name:    "OwnCheckIsSkipped",
			ev:      events.CheckAdded{Header: events.Header{EventID: "e2"}, CheckID: 1, MarkID: 5, UserID: 3},
			getMark: method[models.Mark]{data: models.Mark{ID: 5, UserID: 3}},
		},
		{
			name:    "ErrMarkNotFound",
			ev:      events.CheckAdded{Header: events.Header{EventID: "e3"}, MarkID: 5, UserID: 2},
			getMark: method[models.Mark]{err: repository.ErrNotFound},
			wantErr: true,
		},
		{
			name:       "ErrCreate",
			ev:         events.CheckAdded{Header: events.Header{EventID: "e4"}, MarkID: 5, UserID: 2},
			getMark:    method[models.Mark]{data: models.Mark{ID: 5, UserID: 3}},
			wantCreate: true,
			createErr:  errRepo,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.marks.On("GetMarkById", mock.Anything, tt.ev.MarkID).Once().Return(tt.getMark.data, tt.getMark.err)
			if tt.wantCreate {
				suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
					return n.UserID == tt.getMark.data.UserID &&
						n.EventID == tt.ev.EventID &&
						n.Type == models.NotificationCheckAdded &&
						n.MarkID.ValueOrZero() == int64(tt.ev.MarkID)
				})).Once().Return(int64(1), true, tt.createErr)
			}

			err := suite.uc.HandleCheckAdded(context.Background(), tt.ev)
			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *NotifierSuite) TestHandleOrganizationEvents() {
	orgs := usecase.NewMockNotifierOrganizationsRepository(suite.T())
	uc := usecase.NewNotifier(slogdiscard.NewDiscardLogger(), suite.notifications, usecase.NotifierRepositories{
		Marks:         suite.marks,
		Organizations: orgs,
	})
	dueAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		handle    func(ctx context.Context) error
		eventID   string
		wantType  models.NotificationType
		members   method[[]int]
		createErr error
		wantErr   bool
	}{
		{
			name: "AssignedNotifiesEveryMember",
			handle: func(ctx context.Context) error {
				return uc.HandleMarkAssigned(ctx, events.MarkAssigned{Header: events.Header{EventID: "a1"}, MarkID: 5, OrganizationID: 10, SLADueAt: dueAt})
			},
			eventID:  "a1",
			wantType: models.NotificationMarkAssigned,
			members:  method[[]int]{data: []int{7, 8}},
		},
		{
			name: "SLABreachedNotifiesEveryMember",
			handle: func(ctx context.Context) error {
				return uc.HandleMarkSLABreached(ctx, events.MarkSLABreached{Header: events.Header{EventID: "s1"}, MarkID: 5, OrganizationID: 10, SLADueAt: dueAt})
			},
			eventID:  "s1",
			wantType: models.NotificationMarkSLABreached,
			members:  method[[]int]{data: []int{7}},
		},
		{
			name: "NoMembers",
			handle: func(ctx context.Context) error {
				return uc.HandleMarkAssigned(ctx, events.MarkAssigned{Header: events.Header{EventID: "a2"}, MarkID: 5, OrganizationID: 10})
			},
			members: method[[]int]{data: []int{}},
		},
		{
			name: "ErrMembers",
			handle: func(ctx context.Context) error {
				return uc.HandleMarkAssigned(ctx, events.MarkAssigned{Header: events.Header{EventID: "a3"}, MarkID: 5, OrganizationID: 10})
			},
			members: method[[]int]{err: errRepo},
			wantErr: true,
		},
		{
			name: "ErrCreate",
			handle: func(ctx context.Context) error {
				return uc.HandleMarkAssigned(ctx, events.MarkAssigned{Header: events.Header{EventID: "a4"}, MarkID: 5, OrganizationID: 10})
			},
			eventID:   "a4",
			wantType:  models.NotificationMarkAssigned,
			members:   method[[]int]{data: []int{7}},
			createErr: errRepo,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			orgs.On("GetMemberIDs", mock.Anything, 10).Once().Return(tt.members.data, tt.members.err)
			for _, userID := range tt.members.data {
				suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
					return n.UserID == userID && n.EventID == tt.eventID && n.Type == tt.wantType && n.MarkID.Int64 == 5
				})).Once().Return(int64(1), true, tt.createErr)
				if tt.createErr != nil {
					break
				}
			}

			err := tt.handle(context.Background())
			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.NoError(err)
		})
	}
}
