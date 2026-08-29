//go:build integration

package postgres_test

import (
	"context"
	"errors"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/twpayne/go-geom"
)

func (s *PostgresSuite) TestUsers_GetUserById() {
	tests := []struct {
		name    string
		id      int
		want    models.User
		wantErr error
	}{
		{
			name: "existing user with home point",
			id:   fxUserAlice,
			want: models.User{
				Id: fxUserAlice, Name: "Alice", Login: "alice", PasswordHash: "hash-alice",
				HomePoint: models.NewPoint(coordAliceHome), Rating: 10, Role: models.RoleUser,
			},
		},
		{
			name: "moderator",
			id:   fxUserBob,
			want: models.User{
				Id: fxUserBob, Name: "Bob", Login: "bob", PasswordHash: "hash-bob",
				HomePoint: models.NewPoint(coordBobHome), Rating: 0, Role: models.RoleModerator,
			},
		},
		{name: "missing user", id: 999, wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.users.GetUserById(s.ctx, tt.id)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.assertUser(tt.want, got)
		})
	}
}

func (s *PostgresSuite) TestUsers_GetUserByLogin() {
	tests := []struct {
		name    string
		login   string
		wantID  int
		wantErr error
	}{
		{name: "existing login", login: "alice", wantID: fxUserAlice},
		{name: "login is case sensitive", login: "Alice", wantErr: repository.ErrNotFound},
		{name: "unknown login", login: "nobody", wantErr: repository.ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.users.GetUserByLogin(s.ctx, tt.login)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.wantID, got.Id)
			s.Equal(tt.login, got.Login)
			s.NotEmpty(got.PasswordHash, "password hash must be loaded for login lookups")
		})
	}
}

func (s *PostgresSuite) TestUsers_GetUsers() {
	page, err := s.users.GetUsers(s.ctx, models.Pagination{})
	s.Require().NoError(err)
	users := page.Items
	s.Require().Len(users, 2)
	s.Equal(2, page.Total)

	logins := []string{users[0].Login, users[1].Login}
	s.ElementsMatch([]string{"alice", "bob"}, logins)
	for _, u := range users {
		s.Empty(u.PasswordHash, "GetUsers must not expose password hashes")
		s.NotNil(u.HomePoint)
	}
}

func (s *PostgresSuite) TestUsers_GetUsers_Empty() {
	s.truncate()

	page, err := s.users.GetUsers(s.ctx, models.Pagination{})
	s.Require().NoError(err)
	users := page.Items
	s.NotNil(users)
	s.Equal(0, page.Total)
	s.Empty(users)
}

func (s *PostgresSuite) TestUsers_AddUser() {
	tests := []struct {
		name    string
		user    models.User
		wantErr error
	}{
		{
			name: "new user is inserted with role and geometry",
			user: models.User{
				Name: "Carol", Login: "carol", PasswordHash: "hash-carol",
				HomePoint: models.NewPoint(geom.Coord{41.5, 52.7}), Role: models.RoleAdmin,
			},
		},
		{
			name: "duplicate login violates unique constraint",
			user: models.User{
				Name: "Alice 2", Login: "alice", PasswordHash: "x",
				HomePoint: models.NewPoint(coordAliceHome), Role: models.RoleUser,
			},
			wantErr: repository.ErrExists,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			id, err := s.users.AddUser(s.ctx, tt.user)
			if tt.wantErr != nil {
				s.ErrorIs(err, tt.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Greater(id, int64(fxUserBob))

			got, err := s.users.GetUserById(s.ctx, int(id))
			s.Require().NoError(err)
			tt.user.Id = int(id)
			s.assertUser(tt.user, got)
		})
	}
}

func (s *PostgresSuite) TestUsers_AddUser_InvalidRole() {
	_, err := s.users.AddUser(s.ctx, models.User{
		Name: "Dave", Login: "dave", PasswordHash: "x",
		HomePoint: models.NewPoint(coordAliceHome), Role: models.Role("superuser"),
	})
	s.Require().Error(err)
	s.NotErrorIs(err, repository.ErrExists)
	s.ErrorContains(err, "users_role_check")
}

func (s *PostgresSuite) TestUsers_AddUser_RollbackInTransaction() {
	errAbort := errors.New("abort")

	err := s.trm.Do(s.ctx, func(ctx context.Context) error {
		id, err := s.users.AddUser(ctx, models.User{
			Name: "Ghost", Login: "ghost", PasswordHash: "x",
			HomePoint: models.NewPoint(coordAliceHome), Role: models.RoleUser,
		})
		s.Require().NoError(err)

		// Visible inside the transaction...
		_, err = s.users.GetUserById(ctx, int(id))
		s.Require().NoError(err)
		return errAbort
	})
	s.ErrorIs(err, errAbort)

	// ...but not after rollback.
	_, err = s.users.GetUserByLogin(s.ctx, "ghost")
	s.ErrorIs(err, repository.ErrNotFound)
}

func (s *PostgresSuite) assertUser(want, got models.User) {
	s.Equal(want.Id, got.Id)
	s.Equal(want.Name, got.Name)
	s.Equal(want.Login, got.Login)
	s.Equal(want.PasswordHash, got.PasswordHash)
	s.Equal(want.Rating, got.Rating)
	s.Equal(want.Role, got.Role)
	s.Require().NotNil(got.HomePoint)
	s.InDelta(want.HomePoint.Ewkb.X(), got.HomePoint.Ewkb.X(), 1e-6)
	s.InDelta(want.HomePoint.Ewkb.Y(), got.HomePoint.Ewkb.Y(), 1e-6)
	s.Equal(4326, got.HomePoint.Ewkb.SRID())
}
