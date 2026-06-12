package pg

import (
	"errors"
	"testing"

	"backend/internal/repo"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWrapErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "not found",
			err:  pgx.ErrNoRows,
			want: repo.ErrNotFound,
		},
		{
			name: "unique violation",
			err:  &pgconn.PgError{Code: "23505"},
			want: repo.ErrAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wrapErr(tt.err); !errors.Is(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
