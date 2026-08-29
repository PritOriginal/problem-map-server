package token_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/suite"
)

const (
	testKey  = "test-signing-key-0123456789abcdef"
	otherKey = "another-signing-key-fedcba9876543210"
)

type TokenSuite struct {
	suite.Suite
}

func TestTokenSuite(t *testing.T) {
	suite.Run(t, new(TokenSuite))
}

// parseClaims decodes the token without verifying it, to inspect the payload.
func (s *TokenSuite) parseClaims(tokenString string) jwt.MapClaims {
	claims := jwt.MapClaims{}
	_, _, err := new(jwt.Parser).ParseUnverified(tokenString, claims)
	s.Require().NoError(err)
	return claims
}

func (s *TokenSuite) TestCreateToken() {
	tests := []struct {
		name   string
		ttl    time.Duration
		userID int
		role   string
	}{
		{name: "user", ttl: time.Hour, userID: 1, role: "user"},
		{name: "moderator", ttl: 15 * time.Minute, userID: 42, role: "moderator"},
		{name: "admin", ttl: time.Hour, userID: 3, role: "admin"},
		{name: "empty role", ttl: time.Hour, userID: 7, role: ""},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			before := time.Now().Unix()
			tokenString, err := token.CreateToken(tt.ttl, tt.userID, tt.role, testKey)
			s.Require().NoError(err)
			s.NotEmpty(tokenString)

			claims := s.parseClaims(tokenString)
			s.Equal(strconv.Itoa(tt.userID), claims["sub"])
			s.Equal(tt.role, claims[token.RoleClaim])

			iat := int64(claims["iat"].(float64))
			nbf := int64(claims["nbf"].(float64))
			exp := int64(claims["exp"].(float64))
			s.GreaterOrEqual(iat, before)
			s.LessOrEqual(iat, time.Now().Unix())
			s.Equal(iat, nbf)
			s.Equal(iat+int64(tt.ttl.Seconds()), exp)

			sub, err := token.ValidateToken(tokenString, testKey)
			s.Require().NoError(err)
			s.Equal(strconv.Itoa(tt.userID), sub)
		})
	}
}

func (s *TokenSuite) TestCreateToken_UsesHS256() {
	tokenString, err := token.CreateToken(time.Hour, 1, "user", testKey)
	s.Require().NoError(err)

	parsed, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	s.Require().NoError(err)
	s.Equal(jwt.SigningMethodHS256.Alg(), parsed.Method.Alg())
}

func (s *TokenSuite) TestValidateToken() {
	valid, err := token.CreateToken(time.Hour, 5, "user", testKey)
	s.Require().NoError(err)

	expired, err := token.CreateToken(-time.Minute, 5, "user", testKey)
	s.Require().NoError(err)

	otherKeyToken, err := token.CreateToken(time.Hour, 5, "user", otherKey)
	s.Require().NoError(err)

	// Same claims, but signed with an unsupported (non-HMAC) method.
	noneToken, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "5", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	s.Require().NoError(err)

	// Not yet valid: nbf in the future.
	future := time.Now().Add(time.Hour)
	notYet, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "5", "iat": future.Unix(), "nbf": future.Unix(), "exp": future.Add(time.Hour).Unix(),
	}).SignedString([]byte(testKey))
	s.Require().NoError(err)

	// Token without a subject validates but yields a nil subject.
	noSub, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testKey))
	s.Require().NoError(err)

	tests := []struct {
		name     string
		token    string
		key      string
		wantSub  any
		wantErr  bool
		errMatch string
	}{
		{name: "valid token", token: valid, key: testKey, wantSub: "5"},
		{name: "expired token", token: expired, key: testKey, wantErr: true, errMatch: "expired"},
		{name: "wrong key", token: valid, key: otherKey, wantErr: true, errMatch: "signature is invalid"},
		{name: "token signed with other key", token: otherKeyToken, key: testKey, wantErr: true, errMatch: "signature is invalid"},
		{name: "empty key", token: valid, key: "", wantErr: true},
		{name: "none algorithm is rejected", token: noneToken, key: testKey, wantErr: true, errMatch: "unexpected signing method"},
		{name: "not yet valid", token: notYet, key: testKey, wantErr: true, errMatch: "not valid yet"},
		{name: "tampered payload", token: valid[:len(valid)-3] + "abc", key: testKey, wantErr: true},
		{name: "malformed token", token: "not.a.jwt", key: testKey, wantErr: true},
		{name: "empty token", token: "", key: testKey, wantErr: true},
		{name: "no subject claim", token: noSub, key: testKey, wantErr: true, errMatch: "missing subject"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			sub, err := token.ValidateToken(tt.token, tt.key)
			if tt.wantErr {
				s.Require().Error(err)
				s.ErrorContains(err, "validate:")
				if tt.errMatch != "" {
					s.ErrorContains(err, tt.errMatch)
				}
				s.Nil(sub)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.wantSub, sub)
		})
	}
}

func (s *TokenSuite) TestValidateClaims() {
	tests := []struct {
		name    string
		token   func() string
		key     string
		want    token.Claims
		wantErr bool
	}{
		{
			name: "Ok",
			token: func() string {
				tok, err := token.CreateToken(time.Hour, 42, "admin", testKey)
				s.Require().NoError(err)
				return tok
			},
			key:  testKey,
			want: token.Claims{UserID: 42, Role: "admin"},
		},
		{
			name: "NoRole",
			token: func() string {
				tok, err := token.CreateToken(time.Hour, 1, "", testKey)
				s.Require().NoError(err)
				return tok
			},
			key:  testKey,
			want: token.Claims{UserID: 1},
		},
		{
			name: "WrongKey",
			token: func() string {
				tok, err := token.CreateToken(time.Hour, 1, "user", otherKey)
				s.Require().NoError(err)
				return tok
			},
			key:     testKey,
			wantErr: true,
		},
		{
			name: "Expired",
			token: func() string {
				tok, err := token.CreateToken(-time.Hour, 1, "user", testKey)
				s.Require().NoError(err)
				return tok
			},
			key:     testKey,
			wantErr: true,
		},
		{
			name: "AlgNone",
			token: func() string {
				tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
					"sub": "1", "exp": time.Now().Add(time.Hour).Unix(),
				}).SignedString(jwt.UnsafeAllowNoneSignatureType)
				s.Require().NoError(err)
				return tok
			},
			key:     testKey,
			wantErr: true,
		},
		{
			name:    "Garbage",
			token:   func() string { return "not-a-jwt" },
			key:     testKey,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			claims, err := token.ValidateClaims(tt.token(), tt.key)
			if tt.wantErr {
				s.Error(err)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.want, claims)
		})
	}
}
