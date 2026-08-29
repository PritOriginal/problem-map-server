// Package middleware contains Gin middlewares shared between HTTP handlers.
package middleware

import (
	"fmt"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

// UserIDFromClaims extracts the user id from the "sub" claim of the JWT
// that was validated by gin-jwt middleware.
func UserIDFromClaims(c *gin.Context) (int, error) {
	claims := jwt.ExtractClaims(c)

	sub, err := claims.GetSubject()
	if err != nil {
		return 0, fmt.Errorf("get subject: %w", err)
	}

	userId, err := strconv.Atoi(sub)
	if err != nil {
		return 0, fmt.Errorf("parse subject: %w", err)
	}

	return userId, nil
}

// RoleFromClaims extracts the user role from the JWT claims.
// Tokens without the role claim are treated as plain users.
func RoleFromClaims(c *gin.Context) models.Role {
	claims := jwt.ExtractClaims(c)

	role, _ := claims[token.RoleClaim].(string)

	return models.ParseRole(role)
}

// RequireRole allows the request to continue only if the role from the JWT
// claims is one of the given roles. Must be placed after the JWT middleware.
func RequireRole(roles ...models.Role) gin.HandlerFunc {
	allowed := make(map[models.Role]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *gin.Context) {
		if _, ok := allowed[RoleFromClaims(c)]; !ok {
			responses.Forbidden(c, "forbidden")
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuth reads the JWT when an Authorization header is present and
// records the user as the viewer of the request (models.ContextWithViewer)
// so read endpoints can return per-user fields. Requests without a header,
// or with a token the middleware rejects, continue as anonymous; protected
// routes must still use mw.MiddlewareFunc.
func OptionalAuth(mw *jwt.GinJWTMiddleware) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.Next()
			return
		}

		// GetClaimsFromJWT verifies the signature and the exp/nbf claims;
		// a missing exp is rejected as the strict middleware does.
		claims, err := mw.GetClaimsFromJWT(c)
		if err != nil || claims["exp"] == nil {
			c.Next()
			return
		}
		c.Set("JWT_PAYLOAD", claims)

		if userId, err := UserIDFromClaims(c); err == nil {
			c.Request = c.Request.WithContext(models.ContextWithViewer(c.Request.Context(), userId))
		}

		c.Next()
	}
}
