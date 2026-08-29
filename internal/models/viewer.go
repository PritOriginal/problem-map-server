package models

import "context"

type viewerKey struct{}

// ContextWithViewer records the authenticated user reading the data so that
// repositories can compute per-viewer fields (Mark.IsFollowing). The viewer
// gets the plain user role; use ContextWithActor to record the role too.
func ContextWithViewer(ctx context.Context, userID int) context.Context {
	return ContextWithActor(ctx, Actor{UserID: userID, Role: RoleUser})
}

// ContextWithActor records the authenticated user together with their role
// so that repositories can both compute per-viewer fields and decide what
// the viewer may see (hidden marks are visible to their author and to
// moderators only).
func ContextWithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, viewerKey{}, actor)
}

// ViewerFromContext returns the viewer id or 0 for anonymous requests.
func ViewerFromContext(ctx context.Context) int {
	return ActorFromContext(ctx).UserID
}

// ActorFromContext returns the viewer with their role; the zero Actor
// (UserID 0, empty role) for anonymous requests.
func ActorFromContext(ctx context.Context) Actor {
	actor, _ := ctx.Value(viewerKey{}).(Actor)
	return actor
}
