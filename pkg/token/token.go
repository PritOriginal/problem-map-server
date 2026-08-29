package token

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt"
)

// RoleClaim is the JWT claim name that carries the user role.
const RoleClaim = "role"

// Claims is the set of claims the server puts into an access token.
type Claims struct {
	UserID int
	// Role is the raw role claim; empty when the token carries no role.
	Role string
}

func CreateToken(ttl time.Duration, userId int, role string, key string) (string, error) {
	timeNow := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat":     timeNow.Unix(),
		"nbf":     timeNow.Unix(),
		"exp":     timeNow.Add(ttl).Unix(),
		"sub":     strconv.Itoa(userId),
		RoleClaim: role,
	})
	tokenString, err := token.SignedString([]byte(key))

	if err != nil {
		return "", fmt.Errorf("create: sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates the token and returns its subject (the user id as
// a string). It is kept for callers that only need the subject; see
// ValidateClaims for the parsed claims.
func ValidateToken(tokenString string, key string) (interface{}, error) {
	claims, err := ValidateClaims(tokenString, key)
	if err != nil {
		return nil, err
	}

	return strconv.Itoa(claims.UserID), nil
}

// ValidateClaims verifies the HMAC signature and the standard time claims
// (exp/nbf/iat) and returns the parsed subject and role claims.
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
