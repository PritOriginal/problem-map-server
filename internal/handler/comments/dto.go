package commentsrest

import "github.com/PritOriginal/problem-map-server/internal/models"

// GetCommentsResponse is the payload of GET /marks/{id}/comments.
type GetCommentsResponse struct {
	Comments []models.Comment `json:"comments"`
}

// AddCommentRequest is the JSON body of POST /marks/{id}/comments.
type AddCommentRequest struct {
	// Body is limited to models.MaxCommentBodyLen runes (the binding tag
	// cannot reference the constant; a test keeps them in sync).
	Body string `json:"body" binding:"required,max=2000"`
	// ParentID is the top-level comment replied to; omit for a new thread.
	ParentID *int `json:"parent_id" binding:"omitempty,min=1"`
}

// UpdateCommentRequest is the JSON body of PATCH /comments/{id}.
type UpdateCommentRequest struct {
	Body string `json:"body" binding:"required,max=2000"`
}

// CommentResponse carries one comment (POST, PATCH).
type CommentResponse struct {
	Comment models.Comment `json:"comment"`
}

// DeleteCommentResponse is the payload of DELETE /comments/{id}.
type DeleteCommentResponse struct {
	CommentId int `json:"comment_id"`
}
