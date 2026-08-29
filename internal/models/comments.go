package models

import (
	"time"

	"github.com/guregu/null/v6"
)

// MaxCommentBodyLen is the maximum length (in runes) of a comment body;
// the REST binding tag (max=2000) and the CHECK constraint mirror it.
const MaxCommentBodyLen = 2000

// Comment is a user's comment on a mark. A reply carries ParentID of a
// top-level comment (one level of nesting). A deleted comment stays in
// the thread with Deleted set and an empty Body so that its replies keep
// their parent.
type Comment struct {
	ID       int      `json:"comment_id" db:"comment_id"`
	MarkID   int      `json:"mark_id" db:"mark_id"`
	UserID   int      `json:"user_id" db:"user_id"`
	Username string   `json:"username" db:"username"`
	Body     string   `json:"body" db:"body"`
	ParentID null.Int `json:"parent_id" db:"parent_id" swaggertype:"integer"`
	// Deleted reports a soft-deleted comment (body is empty then).
	Deleted bool `json:"deleted" db:"deleted"`
	// IsMine reports whether the viewer (see ContextWithViewer) wrote the
	// comment; always false for anonymous requests.
	IsMine    bool      `json:"is_mine" db:"is_mine"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
