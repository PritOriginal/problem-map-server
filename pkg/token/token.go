// Package token issues and validates the HS256 JWTs used by the server.
//
// Every token carries the standard iat/nbf/exp/sub claims plus:
//   - "typ": TypeAccess or TypeRefresh, so a refresh token can never be used
//     as a bearer token and vice versa;
//   - "role": the user role at issue time;
//   - "ver": the user's auth version (see usecase.AuthVersionStore); a token
//     whose version is behind the stored one is rejected by the middleware;
//   - "jti": a unique id, set on refresh tokens only, used for one-time
//     rotation and revocation.
package token

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt"
)

// Claim names used on top of the registered JWT claims.
const (
	// RoleClaim is the JWT claim name that carries the user role.
	RoleClaim = "role"
	// TypeClaim distinguishes access tokens from refresh tokens.
	TypeClaim = "typ"
	// VersionClaim carries the auth version the token was issued with.
	VersionClaim = "ver"
	// IDClaim is the registered "jti" claim: the unique id of a refresh token.
	IDClaim = "jti"
)

// Token types stored in the TypeClaim.
const (
	TypeAccess  = "access"
	TypeRefresh = "refresh"
)

// Claims is the set of claims the server puts into a token.
type Claims struct {
	UserID int
	// Role is the raw role claim; empty when the token carries no role.
	Role string
	// Type is TypeAccess or TypeRefresh; empty for legacy tokens.
	Type string
	// Version is the auth version claim; 0 when absent.
	Version int64
	// ID is the "jti" claim; empty when absent.
	ID string
}

// Params describes the token to issue.
type Params struct {
	TTL     time.Duration
	UserID  int
	Role    string
	Type    string
	Version int64
	// ID is put into the "jti" claim when not empty.
	ID string
}

// Create signs a token with the given claims.
func Create(p Params, key string) (string, error) {
	timeNow := time.Now()
	claims := jwt.MapClaims{
		"iat":        timeNow.Unix(),
		"nbf":        timeNow.Unix(),
		"exp":        timeNow.Add(p.TTL).Unix(),
		"sub":        strconv.Itoa(p.UserID),
		RoleClaim:    p.Role,
		TypeClaim:    p.Type,
		VersionClaim: p.Version,
	}
	if p.ID != "" {
		claims[IDClaim] = p.ID
	}

	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(key))
	if err != nil {
		return "", fmt.Errorf("create: sign token: %w", err)
	}

	return tokenString, nil
}

// CreateToken issues an access token (typ=access, ver=0). It is kept for
// callers that need a plain bearer token, such as tests; the auth usecase
// uses Create with the current auth version.
func CreateToken(ttl time.Duration, userId int, role string, key string) (string, error) {
	return Create(Params{TTL: ttl, UserID: userId, Role: role, Type: TypeAccess}, key)
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
// (exp/nbf/iat) and returns the parsed claims. It does not check the token
// type: callers compare Claims.Type with the type they expect.
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

	return ParseClaims(mapClaims)
}

// ParseClaims extracts the server claims from already verified JWT claims
// (e.g. the payload stored by the gin-jwt middleware).
func ParseClaims(mapClaims map[string]any) (Claims, error) {
	sub, ok := mapClaims["sub"].(string)
	if !ok {
		return Claims{}, fmt.Errorf("validate: missing subject")
	}
	userId, err := strconv.Atoi(sub)
	if err != nil {
		return Claims{}, fmt.Errorf("validate: parse subject: %w", err)
	}

	role, _ := mapClaims[RoleClaim].(string)
	typ, _ := mapClaims[TypeClaim].(string)
	id, _ := mapClaims[IDClaim].(string)

	return Claims{
		UserID:  userId,
		Role:    role,
		Type:    typ,
		Version: parseVersion(mapClaims[VersionClaim]),
		ID:      id,
	}, nil
}

// parseVersion reads the "ver" claim, which is a float64 after JSON decoding
// and an int64 when the claims were built in-process.
func parseVersion(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}
