// Package interceptors contains gRPC server interceptors shared between
// the gRPC handlers (authentication and authorization).
package interceptors

import (
	"context"
	"log/slog"
	"strings"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// authorizationHeader is the metadata key carrying the credentials.
	authorizationHeader = "authorization"
	// bearerScheme is the expected scheme of the authorization value.
	bearerScheme = "bearer"
)

type claimsKey struct{}

// Claims is the authenticated caller identity stored in the request context.
type Claims struct {
	UserID int
	Role   models.Role
}

// ContextWithClaims returns a copy of ctx carrying the given claims.
// It is meant for tests and for callers that authenticate by other means.
func ContextWithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// ClaimsFromContext returns the claims put into ctx by the Auth interceptor.
// ok is false when the request is not authenticated.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(Claims)
	return claims, ok
}

// RequireClaims returns the claims put into ctx by the Auth interceptor or
// a codes.Unauthenticated status when the request is anonymous.
func RequireClaims(ctx context.Context) (Claims, error) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return Claims{}, status.Error(codes.Unauthenticated, "authentication required")
	}

	return claims, nil
}

// Auth validates the "authorization: Bearer <jwt>" metadata with the given
// signing key and stores the claims in the context. A request without the
// header is passed through unauthenticated; a request with an invalid token
// is rejected with codes.Unauthenticated regardless of the method. Use
// RequireAuth / RequireRole to protect specific methods.
type Auth struct {
	log *slog.Logger
	key string
}

// NewAuth creates the authentication interceptor with the JWT signing key.
func NewAuth(log *slog.Logger, key string) *Auth {
	return &Auth{log: log, key: key}
}

// AuthFunc is the go-grpc-middleware auth function: it parses the bearer
// token, if any, and stores its claims in the context.
func (a *Auth) AuthFunc(ctx context.Context) (context.Context, error) {
	vals := metadata.ValueFromIncomingContext(ctx, authorizationHeader)
	if len(vals) == 0 {
		// Missing header: the request is anonymous.
		return ctx, nil
	}

	scheme, tokenString, found := strings.Cut(vals[0], " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) || tokenString == "" {
		a.log.Debug("invalid authorization metadata")
		return ctx, status.Error(codes.Unauthenticated, "invalid authorization metadata")
	}

	claims, err := token.ValidateClaims(tokenString, a.key)
	if err != nil {
		a.log.Debug("invalid token", logger.Err(err))
		return ctx, status.Error(codes.Unauthenticated, "invalid token")
	}

	return ContextWithClaims(ctx, Claims{UserID: claims.UserID, Role: models.ParseRole(claims.Role)}), nil
}

// Unary returns the unary interceptor that authenticates every call.
func (a *Auth) Unary() grpc.UnaryServerInterceptor {
	return auth.UnaryServerInterceptor(a.AuthFunc)
}

// Stream returns the stream interceptor that authenticates every call.
func (a *Auth) Stream() grpc.StreamServerInterceptor {
	return auth.StreamServerInterceptor(a.AuthFunc)
}

// MatchMethods returns a selector matcher for the given full method names
// (e.g. pb.Marks_AddMark_FullMethodName).
func MatchMethods(methods ...string) selector.Matcher {
	set := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		set[m] = struct{}{}
	}

	return selector.MatchFunc(func(_ context.Context, c interceptors.CallMeta) bool {
		_, ok := set[c.FullMethod()]
		return ok
	})
}

// RequireAuth rejects unauthenticated calls to the given methods with
// codes.Unauthenticated. Must be placed after Auth.Unary in the chain.
func RequireAuth(methods ...string) grpc.UnaryServerInterceptor {
	return selector.UnaryServerInterceptor(requireAuth, MatchMethods(methods...))
}

func requireAuth(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if _, err := RequireClaims(ctx); err != nil {
		return nil, err
	}

	return handler(ctx, req)
}

// RequireRole allows calls to the given methods only when the caller is
// authenticated and has one of the roles: codes.Unauthenticated without a
// token, codes.PermissionDenied with a wrong role. Must be placed after
// Auth.Unary in the chain.
func RequireRole(roles []models.Role, methods ...string) grpc.UnaryServerInterceptor {
	allowed := make(map[models.Role]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	requireRole := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		claims, err := RequireClaims(ctx)
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[claims.Role]; !ok {
			return nil, status.Error(codes.PermissionDenied, "forbidden")
		}

		return handler(ctx, req)
	}

	return selector.UnaryServerInterceptor(requireRole, MatchMethods(methods...))
}
