package handler

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/usecase"
)

func TestNewMap(t *testing.T) {
	type args struct {
		uc usecase.Map
	}
	tests := []struct {
		name string
		args args
		want *MapHandler
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMap(tt.args.uc); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapHandler_GetDistricts(t *testing.T) {
	tests := []struct {
		name string
		h    *MapHandler
		want http.HandlerFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.GetDistricts(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapHandler.GetDistricts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapHandler_GetMarks(t *testing.T) {
	tests := []struct {
		name string
		h    *MapHandler
		want http.HandlerFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.GetMarks(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapHandler.GetMarks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapHandler_AddMark(t *testing.T) {
	tests := []struct {
		name string
		h    *MapHandler
		want http.HandlerFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.AddMark(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapHandler.AddMark() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapHandler_AddPhotos(t *testing.T) {
	tests := []struct {
		name string
		h    *MapHandler
		want http.HandlerFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.AddPhotos(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapHandler.AddPhotos() = %v, want %v", got, tt.want)
			}
		})
	}
}
