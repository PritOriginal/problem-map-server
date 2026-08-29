package usecase_test

import (
	"context"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// NotifierBadgesSuite covers the badge evaluation the notifier runs on the
// events that change a user's metrics.
type NotifierBadgesSuite struct {
	suite.Suite
	uc            *usecase.Notifier
	notifications *usecase.MockNotificationCreator
	marks         *usecase.MockNotifierMarksRepository
	achievements  *usecase.MockAchievementEvaluator
}

func (suite *NotifierBadgesSuite) SetupTest() {
	suite.notifications = usecase.NewMockNotificationCreator(suite.T())
	suite.marks = usecase.NewMockNotifierMarksRepository(suite.T())
	suite.achievements = usecase.NewMockAchievementEvaluator(suite.T())
	suite.uc = usecase.NewNotifier(slogdiscard.NewDiscardLogger(), suite.notifications, usecase.NotifierRepositories{
		Marks: suite.marks,
	}).WithAchievements(suite.achievements)
}

func TestNotifierBadges(t *testing.T) {
	suite.Run(t, new(NotifierBadgesSuite))
}

func (suite *NotifierBadgesSuite) expectBadgeNotification(userID int, badge models.Badge, err error) {
	suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
		return n.UserID == userID &&
			n.EventID == events.NewBadgeEarned(userID, badge.Code).EventID &&
			n.Type == models.NotificationBadgeEarned &&
			!n.MarkID.Valid && !n.TaskID.Valid &&
			n.Title != "" && n.Body != ""
	})).Once().Return(int64(1), true, err)
}

func (suite *NotifierBadgesSuite) TestHandleTaskCompleted() {
	badge := models.Badge{Code: "helper_5", Name: "Помощник"}
	tests := []struct {
		name    string
		badges  method[[]models.Badge]
		createN int
		err     error
		wantErr bool
	}{
		{name: "OkNoBadges", badges: method[[]models.Badge]{data: []models.Badge{}}},
		{name: "OkBadgeNotified", badges: method[[]models.Badge]{data: []models.Badge{badge}}, createN: 1},
		{name: "ErrEvaluate", badges: method[[]models.Badge]{err: errRepo}, wantErr: true},
		{name: "ErrCreate", badges: method[[]models.Badge]{data: []models.Badge{badge}}, createN: 1, err: errRepo, wantErr: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.achievements.On("Evaluate", mock.Anything, 2).Once().Return(tt.badges.data, tt.badges.err)
			if tt.createN > 0 {
				suite.expectBadgeNotification(2, badge, tt.err)
			}

			err := suite.uc.HandleTaskCompleted(context.Background(), events.NewTaskCompleted(9, 2, 5, 77))
			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *NotifierBadgesSuite) TestHandleCheckAddedEvaluatesChecker() {
	first := models.Badge{Code: "verifier_10", Name: "Проверяющий"}
	second := models.Badge{Code: "streak_7", Name: "Серия"}

	tests := []struct {
		name       string
		markAuthor int
		wantNotify bool
	}{
		{name: "OtherAuthorNotifiedAndCheckerEvaluated", markAuthor: 3, wantNotify: true},
		{name: "OwnMarkStillEvaluated", markAuthor: 2},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			ev := events.NewCheckAdded(77, 5, 2)
			suite.marks.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{ID: 5, UserID: tt.markAuthor}, nil)
			if tt.wantNotify {
				suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
					return n.UserID == 3 && n.Type == models.NotificationCheckAdded && n.EventID == ev.EventID
				})).Once().Return(int64(1), true, nil)
			}
			suite.achievements.On("Evaluate", mock.Anything, 2).Once().Return([]models.Badge{first, second}, nil)
			suite.expectBadgeNotification(2, first, nil)
			suite.expectBadgeNotification(2, second, nil)

			suite.NoError(suite.uc.HandleCheckAdded(context.Background(), ev))
		})
	}
}

func (suite *NotifierBadgesSuite) TestHandleMarkStatusChangedEvaluatesAuthor() {
	ev := events.NewMarkStatusChanged(5, models.UnderReviewStatus, models.ClosedStatus, 3)
	badge := models.Badge{Code: "resolver", Name: "Решатель"}

	suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
		return n.UserID == 3 && n.Type == models.NotificationMarkStatusChanged
	})).Once().Return(int64(1), true, nil)
	suite.achievements.On("Evaluate", mock.Anything, 3).Once().Return([]models.Badge{badge}, nil)
	suite.expectBadgeNotification(3, badge, nil)

	suite.NoError(suite.uc.HandleMarkStatusChanged(context.Background(), ev))
}

func (suite *NotifierBadgesSuite) TestWithoutEvaluatorNothingIsAwarded() {
	uc := usecase.NewNotifier(slogdiscard.NewDiscardLogger(), suite.notifications, usecase.NotifierRepositories{Marks: suite.marks})
	suite.NoError(uc.HandleTaskCompleted(context.Background(), events.NewTaskCompleted(9, 2, 5, 77)))
}
