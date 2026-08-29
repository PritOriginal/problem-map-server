package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func TestWrapPgError(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "Nil", err: nil, want: nil},
		{name: "NoRows", err: sql.ErrNoRows, want: repository.ErrNotFound},
		{name: "WrappedNoRows", err: fmt.Errorf("get: %w", sql.ErrNoRows), want: repository.ErrNotFound},
		{name: "UniqueViolation", err: &pq.Error{Code: uniqueViolationCode}, want: repository.ErrExists},
		{name: "ForeignKeyViolation", err: &pq.Error{Code: foreignKeyViolationCode}, want: repository.ErrInvalidReference},
		{name: "OtherPgError", err: &pq.Error{Code: "42P01"}, want: nil},
		{name: "Other", err: errBoom, want: errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapPgError("op", tt.err)
			if tt.err == nil {
				assert.NoError(t, got)
				return
			}
			assert.ErrorContains(t, got, "op: ")
			if tt.want != nil {
				assert.ErrorIs(t, got, tt.want)
			} else {
				assert.ErrorIs(t, got, tt.err)
				assert.NotErrorIs(t, got, repository.ErrNotFound)
				assert.NotErrorIs(t, got, repository.ErrExists)
				assert.NotErrorIs(t, got, repository.ErrInvalidReference)
			}
		})
	}
}
