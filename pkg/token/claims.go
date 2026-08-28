package token

import (
	"fmt"
	"strconv"

	"github.com/golang-jwt/jwt"
)

// Claims is the set of claims the server puts into an access token.
type Claims struct {
	UserID int
	// Role is the raw role claim; empty when the token carries no role.
	Role string
}

// ValidateClaims validates the token the same way ValidateToken does and
// returns the parsed subject and role claims.
func ValidateClaims(tokenString string, key string) (Claims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(key), nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("validate: %w", err)
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return Claims{}, fmt.Errorf("validate: invalid token")
	}

	sub, ok := mapClaims["sub"].(string)
	if !ok {
		return Claims{}, fmt.Errorf("validate: missing subject")
	}
	userId, err := strconv.Atoi(sub)
	if err != nil {
		return Claims{}, fmt.Errorf("validate: parse subject: %w", err)
	}

	role, _ := mapClaims[RoleClaim].(string)

	return Claims{UserID: userId, Role: role}, nil
}
