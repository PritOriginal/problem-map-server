package usecase

import (
	"context"
	"reflect"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/db"
)

func TestMapUseCase_GetRegions(t *testing.T) {
	type fields struct {
		mapRepo db.MapRepository
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []models.Region
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &MapUseCase{
				mapRepo: tt.fields.mapRepo,
			}
			got, err := uc.GetRegions(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapUseCase.GetRegions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapUseCase.GetRegions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapUseCase_GetDistricts(t *testing.T) {
	type fields struct {
		mapRepo db.MapRepository
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []models.District
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &MapUseCase{
				mapRepo: tt.fields.mapRepo,
			}
			got, err := uc.GetDistricts(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapUseCase.GetDistricts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapUseCase.GetDistricts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapUseCase_GetMarks(t *testing.T) {
	type fields struct {
		mapRepo db.MapRepository
	}
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []models.Mark
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &MapUseCase{
				mapRepo: tt.fields.mapRepo,
			}
			got, err := uc.GetMarks(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapUseCase.GetMarks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapUseCase.GetMarks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapUseCase_AddMark(t *testing.T) {
	type fields struct {
		mapRepo db.MapRepository
	}
	type args struct {
		ctx  context.Context
		mark models.Mark
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &MapUseCase{
				mapRepo: tt.fields.mapRepo,
			}
			if err := uc.AddMark(tt.args.ctx, tt.args.mark); (err != nil) != tt.wantErr {
				t.Errorf("MapUseCase.AddMark() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
