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

	role, ok := claims[token.RoleClaim].(string)
	if !ok || role == "" {
		return models.RoleUser
	}

	return models.Role(role)
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
