package token

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt"
)

func CreateToken(ttl time.Duration, userId int, key string) (string, error) {
	timeNow := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat": timeNow.Unix(),
		"nbf": timeNow.Unix(),
		"exp": timeNow.Add(ttl).Unix(),
		"sub": strconv.Itoa(userId),
	})
	tokenString, err := token.SignedString([]byte(key))

	if err != nil {
		return "", fmt.Errorf("create: sign token: %w", err)
	}

	return tokenString, nil
}

func ValidateToken(tokenString string, key string) (interface{}, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(key), nil
	})

	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("validate: invalid token")
	}

	return claims["sub"], nil
}
