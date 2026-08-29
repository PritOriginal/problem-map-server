package models

import "context"

type viewerKey struct{}

// ContextWithViewer records the authenticated user reading the data so that
// repositories can compute per-viewer fields (Mark.IsFollowing).
func ContextWithViewer(ctx context.Context, userID int) context.Context {
	return context.WithValue(ctx, viewerKey{}, userID)
}

// ViewerFromContext returns the viewer id or 0 for anonymous requests.
func ViewerFromContext(ctx context.Context) int {
	id, _ := ctx.Value(viewerKey{}).(int)
	return id
}
