package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

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

const (
	commentsMaxPerDay = 3
	commentsEditWin   = 15 * time.Minute
)

type CommentsSuite struct {
	suite.Suite
	uc       *usecase.Comments
	comments *usecase.MockCommentsRepository
	marks    *usecase.MockCommentsMarksRepository
	pub      *events.MockPublisher
}

func (suite *CommentsSuite) SetupTest() {
	suite.comments = usecase.NewMockCommentsRepository(suite.T())
	suite.marks = usecase.NewMockCommentsMarksRepository(suite.T())
	suite.pub = events.NewMockPublisher(suite.T())
	suite.uc = usecase.NewComments(slogdiscard.NewDiscardLogger(),
		config.CommentsConfig{EditWindow: commentsEditWin, MaxPerDay: commentsMaxPerDay},
		usecase.CommentsRepositories{Comments: suite.comments, Marks: suite.marks},
	).WithEvents(suite.pub)
}

func TestComments(t *testing.T) {
	suite.Run(t, new(CommentsSuite))
}

func (suite *CommentsSuite) TestListComments() {
	tests := []struct {
		name    string
		p       models.Pagination
		getMark *method[models.Mark]
		list    *method[models.Page[models.Comment]]
		wantErr error
	}{
		{
			name:    "Ok",
			p:       models.Pagination{Limit: 10},
			getMark: &method[models.Mark]{data: models.Mark{ID: 1}},
			list:    &method[models.Page[models.Comment]]{data: models.Page[models.Comment]{Items: []models.Comment{{ID: 1}}, Total: 1}},
		},
		{name: "ErrInvalidPagination", p: models.Pagination{Limit: -1}, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrMarkNotFound", getMark: &method[models.Mark]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{
			name:    "ErrRepo",
			getMark: &method[models.Mark]{data: models.Mark{ID: 1}},
			list:    &method[models.Page[models.Comment]]{err: errRepo},
			wantErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getMark != nil {
				suite.marks.On("GetMarkById", mock.Anything, 1).Once().Return(tt.getMark.data, tt.getMark.err)
			}
			if tt.list != nil {
				suite.comments.On("GetCommentsByMarkId", mock.Anything, 1, tt.p).Once().Return(tt.list.data, tt.list.err)
			}

			got, err := suite.uc.ListComments(context.Background(), 1, tt.p)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.list.data, got)
		})
	}
}

func (suite *CommentsSuite) TestAddComment() {
	const (
		markID    = 1
		userID    = 2
		authorID  = 7
		parentID  = 5
		commentID = 10
	)
	parent := parentID
	mark := &method[models.Mark]{data: models.Mark{ID: markID, UserID: authorID}}

	tests := []struct {
		name      string
		body      string
		parentID  *int
		getMark   *method[models.Mark]
		getParent *method[models.Comment]
		count     *method[int]
		dup       *method[bool]
		add       *method[int64]
		getAdded  *method[models.Comment]
		// wantBody is the trimmed body the repository must receive.
		wantBody string
		wantErr  error
	}{
		{
			name:     "OkTrimmed",
			body:     "  hello  ",
			getMark:  mark,
			count:    &method[int]{data: 0},
			dup:      &method[bool]{data: false},
			add:      &method[int64]{data: commentID},
			getAdded: &method[models.Comment]{data: models.Comment{ID: commentID, MarkID: markID, UserID: userID, Body: "hello"}},
			wantBody: "hello",
		},
		{
			name:     "OkMultiline",
			body:     "line 1\r\n\tline 2",
			getMark:  mark,
			count:    &method[int]{data: 0},
			dup:      &method[bool]{data: false},
			add:      &method[int64]{data: commentID},
			getAdded: &method[models.Comment]{data: models.Comment{ID: commentID, MarkID: markID, UserID: userID, Body: "line 1\r\n\tline 2"}},
			wantBody: "line 1\r\n\tline 2",
		},
		{
			name:      "OkReply",
			body:      "reply",
			parentID:  &parent,
			getMark:   mark,
			getParent: &method[models.Comment]{data: models.Comment{ID: parentID, MarkID: markID}},
			count:     &method[int]{data: commentsMaxPerDay - 1},
			dup:       &method[bool]{data: false},
			add:       &method[int64]{data: commentID},
			getAdded:  &method[models.Comment]{data: models.Comment{ID: commentID, MarkID: markID, UserID: userID, Body: "reply", ParentID: null.IntFrom(parentID)}},
			wantBody:  "reply",
		},
		{name: "ErrEmptyBody", body: "  \n\t ", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrTooLong", body: strings.Repeat("я", models.MaxCommentBodyLen+1), wantErr: usecase.ErrInvalidArgument},
		{name: "ErrNulByte", body: "hi\x00there", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrControlChar", body: "hi\x1bthere", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrInvalidUTF8", body: "hi\xff", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrMarkNotFound", body: "hi", getMark: &method[models.Mark]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{
			name: "ErrMarkHidden", body: "hi",
			getMark: &method[models.Mark]{data: models.Mark{ID: markID, UserID: authorID, Hidden: true}},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrMarkMerged", body: "hi",
			getMark: &method[models.Mark]{data: models.Mark{ID: markID, UserID: authorID, MergedIntoID: null.IntFrom(99)}},
			wantErr: usecase.ErrConflict,
		},
		{
			name: "ErrParentNotFound", body: "hi", parentID: &parent, getMark: mark,
			getParent: &method[models.Comment]{err: repository.ErrNotFound},
			wantErr:   usecase.ErrInvalidArgument,
		},
		{
			name: "ErrParentOtherMark", body: "hi", parentID: &parent, getMark: mark,
			getParent: &method[models.Comment]{data: models.Comment{ID: parentID, MarkID: markID + 1}},
			wantErr:   usecase.ErrInvalidArgument,
		},
		{
			name: "ErrParentIsReply", body: "hi", parentID: &parent, getMark: mark,
			getParent: &method[models.Comment]{data: models.Comment{ID: parentID, MarkID: markID, ParentID: null.IntFrom(3)}},
			wantErr:   usecase.ErrInvalidArgument,
		},
		{
			name: "ErrParentDeleted", body: "hi", parentID: &parent, getMark: mark,
			getParent: &method[models.Comment]{data: models.Comment{ID: parentID, MarkID: markID, Deleted: true}},
			wantErr:   usecase.ErrConflict,
		},
		{
			name: "ErrParentRepo", body: "hi", parentID: &parent, getMark: mark,
			getParent: &method[models.Comment]{err: errRepo},
			wantErr:   errRepo,
		},
		{
			name: "ErrDailyLimit", body: "hi", getMark: mark,
			count:   &method[int]{data: commentsMaxPerDay},
			wantErr: usecase.ErrTooManyRequests,
		},
		{
			name: "ErrDuplicate", body: "hi", getMark: mark,
			count:    &method[int]{data: 0},
			dup:      &method[bool]{data: true},
			wantBody: "hi",
			wantErr:  usecase.ErrConflict,
		},
		{
			name: "ErrAddRepo", body: "hi", getMark: mark,
			count:    &method[int]{data: 0},
			dup:      &method[bool]{data: false},
			add:      &method[int64]{err: errRepo},
			wantBody: "hi",
			wantErr:  errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getMark != nil {
				suite.marks.On("GetMarkById", mock.Anything, markID).Once().Return(tt.getMark.data, tt.getMark.err)
			}
			if tt.getParent != nil {
				suite.comments.On("GetCommentById", mock.Anything, parentID).Once().Return(tt.getParent.data, tt.getParent.err)
			}
			if tt.count != nil {
				suite.comments.On("CountCommentsByUserIdSince", mock.Anything, userID, mock.AnythingOfType("time.Time")).Once().
					Return(tt.count.data, tt.count.err)
			}
			if tt.dup != nil {
				suite.comments.On("HasRecentDuplicate", mock.Anything, userID, markID, tt.wantBody, mock.AnythingOfType("time.Time")).Once().
					Return(tt.dup.data, tt.dup.err)
			}
			if tt.add != nil {
				suite.comments.On("AddComment", mock.Anything, mock.MatchedBy(func(c models.Comment) bool {
					return c.MarkID == markID && c.UserID == userID && c.Body == tt.wantBody &&
						c.ParentID.Valid == (tt.parentID != nil)
				})).Once().Return(tt.add.data, tt.add.err)
			}
			if tt.getAdded != nil {
				suite.comments.On("GetCommentById", mock.Anything, commentID).Once().Return(tt.getAdded.data, tt.getAdded.err)
				suite.pub.On("Publish", mock.Anything, events.SubjectCommentAdded, mock.MatchedBy(func(ev events.CommentAdded) bool {
					sameParent := (ev.ParentID == nil) == (tt.parentID == nil) && (ev.ParentID == nil || *ev.ParentID == *tt.parentID)
					return ev.EventID != "" && ev.CommentID == commentID && ev.MarkID == markID &&
						ev.UserID == userID && ev.AuthorID == authorID && sameParent
				})).Once().Return(nil)
			}

			comment := models.Comment{MarkID: markID, UserID: userID, Body: tt.body}
			if tt.parentID != nil {
				comment.ParentID = null.IntFrom(int64(*tt.parentID))
			}
			got, err := suite.uc.AddComment(context.Background(), comment)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.getAdded.data, got)
		})
	}
}

func (suite *CommentsSuite) TestUpdateComment() {
	const commentID = 10
	owner := models.Actor{UserID: 1, Role: models.RoleUser}
	fresh := time.Now().Add(-time.Minute)
	stale := time.Now().Add(-commentsEditWin - time.Minute)

	tests := []struct {
		name     string
		actor    models.Actor
		body     string
		get      *method[models.Comment]
		update   *method[struct{}]
		getAfter *method[models.Comment]
		wantErr  error
	}{
		{
			name:     "OkOwner",
			actor:    owner,
			body:     " fixed ",
			get:      &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, CreatedAt: fresh}},
			update:   &method[struct{}]{},
			getAfter: &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, Body: "fixed", CreatedAt: fresh}},
		},
		{name: "ErrEmptyBody", actor: owner, body: "   ", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrNotFound", actor: owner, body: "x", get: &method[models.Comment]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{
			name:    "ErrForbiddenStranger",
			actor:   models.Actor{UserID: 2, Role: models.RoleUser},
			body:    "x",
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, CreatedAt: fresh}},
			wantErr: usecase.ErrForbidden,
		},
		{
			// Moderators may delete, not rewrite, other people's words.
			name:    "ErrForbiddenModerator",
			actor:   models.Actor{UserID: 2, Role: models.RoleModerator},
			body:    "x",
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, CreatedAt: fresh}},
			wantErr: usecase.ErrForbidden,
		},
		{
			name:    "ErrDeleted",
			actor:   owner,
			body:    "x",
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, CreatedAt: fresh, Deleted: true}},
			wantErr: usecase.ErrConflict,
		},
		{
			name:    "ErrEditWindowExpired",
			actor:   owner,
			body:    "x",
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, CreatedAt: stale}},
			wantErr: usecase.ErrConflict,
		},
		{
			name:    "ErrUpdateRepo",
			actor:   owner,
			body:    "x",
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, CreatedAt: fresh}},
			update:  &method[struct{}]{err: errRepo},
			wantErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.get != nil {
				suite.comments.On("GetCommentById", mock.Anything, commentID).Once().Return(tt.get.data, tt.get.err)
			}
			if tt.update != nil {
				suite.comments.On("UpdateCommentBody", mock.Anything, commentID, strings.TrimSpace(tt.body)).Once().Return(tt.update.err)
			}
			if tt.getAfter != nil {
				suite.comments.On("GetCommentById", mock.Anything, commentID).Once().Return(tt.getAfter.data, tt.getAfter.err)
			}

			got, err := suite.uc.UpdateComment(context.Background(), tt.actor, commentID, tt.body)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.getAfter.data, got)
		})
	}
}

func (suite *CommentsSuite) TestDeleteComment() {
	const commentID = 10

	tests := []struct {
		name    string
		actor   models.Actor
		get     *method[models.Comment]
		del     *method[struct{}]
		wantErr error
	}{
		{
			name:  "OkOwner",
			actor: models.Actor{UserID: 1, Role: models.RoleUser},
			get:   &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1}},
			del:   &method[struct{}]{},
		},
		{
			name:  "OkModerator",
			actor: models.Actor{UserID: 2, Role: models.RoleModerator},
			get:   &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1}},
			del:   &method[struct{}]{},
		},
		{
			name:  "OkAdmin",
			actor: models.Actor{UserID: 2, Role: models.RoleAdmin},
			get:   &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1}},
			del:   &method[struct{}]{},
		},
		{
			name:    "ErrForbiddenStranger",
			actor:   models.Actor{UserID: 2, Role: models.RoleUser},
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1}},
			wantErr: usecase.ErrForbidden,
		},
		{name: "ErrNotFound", actor: models.Actor{UserID: 1}, get: &method[models.Comment]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{
			name:    "ErrAlreadyDeleted",
			actor:   models.Actor{UserID: 1, Role: models.RoleUser},
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1, Deleted: true}},
			wantErr: usecase.ErrConflict,
		},
		{
			name:    "ErrDeleteRepo",
			actor:   models.Actor{UserID: 1, Role: models.RoleUser},
			get:     &method[models.Comment]{data: models.Comment{ID: commentID, UserID: 1}},
			del:     &method[struct{}]{err: errRepo},
			wantErr: errRepo,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.comments.On("GetCommentById", mock.Anything, commentID).Once().Return(tt.get.data, tt.get.err)
			if tt.del != nil {
				suite.comments.On("SoftDeleteComment", mock.Anything, commentID).Once().Return(tt.del.err)
			}

			err := suite.uc.DeleteComment(context.Background(), tt.actor, commentID)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
		})
	}
}
