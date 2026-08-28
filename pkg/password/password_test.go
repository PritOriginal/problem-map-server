package password_test

import (
	"strings"
	"testing"

	"github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
)

type PasswordSuite struct {
	suite.Suite
}

func TestPasswordSuite(t *testing.T) {
	suite.Run(t, new(PasswordSuite))
}

func (s *PasswordSuite) TestHashPassword() {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "regular password", password: "S3cret!pass"},
		{name: "empty password", password: ""},
		{name: "unicode password", password: "пароль-密码-🔒"},
		{name: "72 bytes is the bcrypt limit", password: strings.Repeat("a", 72)},
		{name: "longer than 72 bytes is rejected", password: strings.Repeat("a", 73), wantErr: true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			hash, err := password.HashPassword(tt.password)
			if tt.wantErr {
				s.Require().Error(err)
				s.ErrorIs(err, bcrypt.ErrPasswordTooLong)
				return
			}
			s.Require().NoError(err)
			s.True(strings.HasPrefix(hash, "$2a$"), "bcrypt hash prefix, got %q", hash)
			s.Len(hash, 60)

			cost, err := bcrypt.Cost([]byte(hash))
			s.Require().NoError(err)
			s.Equal(bcrypt.DefaultCost, cost)

			s.True(password.CheckPasswordHash(tt.password, hash))
		})
	}
}

func (s *PasswordSuite) TestHashPassword_IsSalted() {
	first, err := password.HashPassword("same")
	s.Require().NoError(err)
	second, err := password.HashPassword("same")
	s.Require().NoError(err)

	s.NotEqual(first, second, "each hash must use a fresh salt")
	s.True(password.CheckPasswordHash("same", first))
	s.True(password.CheckPasswordHash("same", second))
}

func (s *PasswordSuite) TestCheckPasswordHash() {
	hash, err := password.HashPassword("correct horse")
	s.Require().NoError(err)

	// A hash produced with a non-default cost must still verify.
	lowCost, err := bcrypt.GenerateFromPassword([]byte("cheap"), bcrypt.MinCost)
	s.Require().NoError(err)

	tests := []struct {
		name     string
		password string
		hash     string
		want     bool
	}{
		{name: "matching password", password: "correct horse", hash: hash, want: true},
		{name: "wrong password", password: "correct horsE", hash: hash, want: false},
		{name: "empty password against real hash", password: "", hash: hash, want: false},
		{name: "empty hash", password: "correct horse", hash: "", want: false},
		{name: "garbage hash", password: "correct horse", hash: "not-a-bcrypt-hash", want: false},
		{name: "truncated hash", password: "correct horse", hash: hash[:30], want: false},
		{name: "min cost hash", password: "cheap", hash: string(lowCost), want: true},
		{name: "min cost hash wrong password", password: "cheaper", hash: string(lowCost), want: false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.want, password.CheckPasswordHash(tt.password, tt.hash))
		})
	}
}
