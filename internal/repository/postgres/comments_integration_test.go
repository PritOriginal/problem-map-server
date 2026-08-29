//go:build integration

package postgres_test

import (
	"context"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/guregu/null/v6"
)

// addComment inserts a comment through the repository and returns its id.
func (s *PostgresSuite) addComment(markID, userID int, body string, parentID int) int {
	c := models.Comment{MarkID: markID, UserID: userID, Body: body}
	if parentID > 0 {
		c.ParentID = null.IntFrom(int64(parentID))
	}
	id, err := s.comments.AddComment(s.ctx, c)
	s.Require().NoError(err)
	return int(id)
}

func (s *PostgresSuite) TestComments_AddAndGet() {
	id := s.addComment(fxMarkNear, fxUserBob, "Согласен, свалка", 0)
	reply := s.addComment(fxMarkNear, fxUserAlice, "Спасибо", id)

	tests := []struct {
		name   string
		id     int
		viewer int
		want   models.Comment
	}{
		{
			name: "top-level, anonymous viewer", id: id,
			want: models.Comment{ID: id, MarkID: fxMarkNear, UserID: fxUserBob, Username: "Bob", Body: "Согласен, свалка"},
		},
		{
			name: "reply, author is the viewer", id: reply, viewer: fxUserAlice,
			want: models.Comment{ID: reply, MarkID: fxMarkNear, UserID: fxUserAlice, Username: "Alice", Body: "Спасибо", ParentID: null.IntFrom(int64(id)), IsMine: true},
		},
		{
			name: "reply, other viewer", id: reply, viewer: fxUserBob,
			want: models.Comment{ID: reply, MarkID: fxMarkNear, UserID: fxUserAlice, Username: "Alice", Body: "Спасибо", ParentID: null.IntFrom(int64(id))},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			ctx := s.ctx
			if tt.viewer != 0 {
				ctx = models.ContextWithViewer(ctx, tt.viewer)
			}
			got, err := s.comments.GetCommentById(ctx, tt.id)
			s.Require().NoError(err)
			s.False(got.CreatedAt.IsZero())
			s.False(got.UpdatedAt.IsZero())
			got.CreatedAt, got.UpdatedAt = time.Time{}, time.Time{}
			s.Equal(tt.want, got)
		})
	}

	_, err := s.comments.GetCommentById(s.ctx, 404)
	s.ErrorIs(err, repository.ErrNotFound)
}

func (s *PostgresSuite) TestComments_AddInvalidReference() {
	tests := []struct {
		name    string
		comment models.Comment
	}{
		{name: "unknown mark", comment: models.Comment{MarkID: 404, UserID: fxUserAlice, Body: "x"}},
		{name: "unknown user", comment: models.Comment{MarkID: fxMarkNear, UserID: 404, Body: "x"}},
		{name: "unknown parent", comment: models.Comment{MarkID: fxMarkNear, UserID: fxUserAlice, Body: "x", ParentID: null.IntFrom(404)}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := s.comments.AddComment(s.ctx, tt.comment)
			s.ErrorIs(err, repository.ErrInvalidReference)
		})
	}
}

func (s *PostgresSuite) TestComments_ListPaginationAndDeleted() {
	var created []int
	for i, body := range []string{"first", "second", "third", "fourth"} {
		created = append(created, s.addComment(fxMarkNear, fxUserBob, body, 0))
		// Distinct timestamps so that the order is by created_at, not by id only.
		_, err := s.db.ExecContext(s.ctx, "UPDATE mark_comments SET created_at = NOW() - ($1 || ' minutes')::interval WHERE comment_id = $2", 10-i, created[i])
		s.Require().NoError(err)
	}
	reply := s.addComment(fxMarkNear, fxUserAlice, "reply to second", created[1])
	s.addComment(fxMarkInside, fxUserBob, "other mark", 0)

	// Soft-delete the parent of the reply: it stays in the thread, blank.
	s.Require().NoError(s.comments.SoftDeleteComment(s.ctx, created[1]))

	tests := []struct {
		name      string
		p         models.Pagination
		wantIDs   []int
		wantTotal int
	}{
		{name: "all, oldest first", wantIDs: []int{created[0], created[1], created[2], created[3], reply}, wantTotal: 5},
		{name: "first page", p: models.Pagination{Limit: 2}, wantIDs: []int{created[0], created[1]}, wantTotal: 5},
		{name: "second page", p: models.Pagination{Limit: 2, Offset: 2}, wantIDs: []int{created[2], created[3]}, wantTotal: 5},
		{name: "beyond the end keeps the total", p: models.Pagination{Limit: 2, Offset: 10}, wantIDs: []int{}, wantTotal: 5},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			page, err := s.comments.GetCommentsByMarkId(models.ContextWithViewer(s.ctx, fxUserBob), fxMarkNear, tt.p)
			s.Require().NoError(err)
			s.Equal(tt.wantTotal, page.Total)
			s.Equal(tt.wantIDs, ids(page.Items, func(c models.Comment) int { return c.ID }))
			for _, c := range page.Items {
				s.Equal(c.UserID == fxUserBob, c.IsMine)
				if c.ID == created[1] {
					s.True(c.Deleted)
					s.Empty(c.Body)
				} else {
					s.False(c.Deleted)
					s.NotEmpty(c.Body)
				}
				if c.ID == reply {
					s.Equal(int64(created[1]), c.ParentID.ValueOrZero())
				}
			}
		})
	}

	// Only live comments count.
	mark, err := s.marks.GetMarkById(s.ctx, fxMarkNear)
	s.Require().NoError(err)
	s.Equal(4, mark.CommentsCount)

	// A second delete of the same comment and a delete of a missing one are ErrNotFound.
	s.ErrorIs(s.comments.SoftDeleteComment(s.ctx, created[1]), repository.ErrNotFound)
	s.ErrorIs(s.comments.SoftDeleteComment(s.ctx, 404), repository.ErrNotFound)
}

func (s *PostgresSuite) TestComments_UpdateBody() {
	id := s.addComment(fxMarkNear, fxUserBob, "typo", 0)
	before, err := s.comments.GetCommentById(s.ctx, id)
	s.Require().NoError(err)

	s.Require().NoError(s.comments.UpdateCommentBody(s.ctx, id, "fixed"))
	after, err := s.comments.GetCommentById(s.ctx, id)
	s.Require().NoError(err)
	s.Equal("fixed", after.Body)
	s.True(after.UpdatedAt.After(before.UpdatedAt) || after.UpdatedAt.Equal(before.UpdatedAt))
	s.Equal(before.CreatedAt, after.CreatedAt)

	s.Require().NoError(s.comments.SoftDeleteComment(s.ctx, id))
	s.ErrorIs(s.comments.UpdateCommentBody(s.ctx, id, "again"), repository.ErrNotFound)
	s.ErrorIs(s.comments.UpdateCommentBody(s.ctx, 404, "x"), repository.ErrNotFound)
}

func (s *PostgresSuite) TestComments_Limits() {
	first := s.addComment(fxMarkNear, fxUserBob, "same text", 0)
	s.addComment(fxMarkInside, fxUserBob, "same text", 0)
	s.addComment(fxMarkNear, fxUserAlice, "same text", 0)
	// A deleted comment still counts against the limits.
	s.Require().NoError(s.comments.SoftDeleteComment(s.ctx, first))

	n, err := s.comments.CountCommentsByUserIdSince(s.ctx, fxUserBob, time.Now().Add(-time.Hour))
	s.Require().NoError(err)
	s.Equal(2, n)
	n, err = s.comments.CountCommentsByUserIdSince(s.ctx, fxUserBob, time.Now().Add(time.Hour))
	s.Require().NoError(err)
	s.Equal(0, n)

	tests := []struct {
		name   string
		userID int
		markID int
		body   string
		since  time.Time
		want   bool
	}{
		{name: "same user, mark and body", userID: fxUserBob, markID: fxMarkNear, body: "same text", since: time.Now().Add(-time.Minute), want: true},
		{name: "other body", userID: fxUserBob, markID: fxMarkNear, body: "other text", since: time.Now().Add(-time.Minute), want: false},
		{name: "other mark", userID: fxUserBob, markID: fxMarkFar, body: "same text", since: time.Now().Add(-time.Minute), want: false},
		{name: "outside the window", userID: fxUserBob, markID: fxMarkNear, body: "same text", since: time.Now().Add(time.Minute), want: false},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.comments.HasRecentDuplicate(s.ctx, tt.userID, tt.markID, tt.body, tt.since)
			s.Require().NoError(err)
			s.Equal(tt.want, got)
		})
	}
}

func (s *PostgresSuite) TestComments_CascadeOnMarkDelete() {
	id := s.addComment(fxMarkInside, fxUserBob, "to be cascaded", 0)
	s.addComment(fxMarkInside, fxUserAlice, "reply", id)
	keep := s.addComment(fxMarkNear, fxUserBob, "stays", 0)

	err := s.trm.Do(s.ctx, func(ctx context.Context) error {
		return s.marks.DeleteMark(ctx, fxMarkInside)
	})
	s.Require().NoError(err)

	s.Equal(0, s.countRows("mark_comments", "mark_id = $1", fxMarkInside))
	s.Equal(1, s.countRows("mark_comments", "comment_id = $1", keep))
}
