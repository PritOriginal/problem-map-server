package handler

import (
	"log/slog"
	"reflect"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

func TestGetRoute(t *testing.T) {
	type args struct {
		log    *slog.Logger
		dbConn *sqlx.DB
	}
	tests := []struct {
		name string
		args args
		want *chi.Mux
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetRoute(tt.args.log, tt.args.dbConn); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}
