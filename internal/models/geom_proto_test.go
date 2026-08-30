package models

import (
	"testing"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
)

func TestPoint_ToProtobufObject(t *testing.T) {
	tests := []struct {
		name  string
		point *Point
		want  *pb.Point
	}{
		{
			name:  "Nil",
			point: nil,
			want:  nil,
		},
		{
			name:  "Empty",
			point: &Point{},
			want:  nil,
		},
		{
			name:  "Ok",
			point: NewPoint(geom.Coord{41.463077, 52.718319}),
			want: &pb.Point{
				Type:        "Point",
				Coordinates: &pb.Coordinates{Longitude: 41.463077, Latitude: 52.718319},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.point.ToProtobufObject())
		})
	}
}

func TestPolygon_ToProtobufObject(t *testing.T) {
	tests := []struct {
		name    string
		polygon *Polygon
		want    *pb.Polygon
	}{
		{
			name:    "Nil",
			polygon: nil,
			want:    nil,
		},
		{
			name:    "Empty",
			polygon: &Polygon{},
			want:    nil,
		},
		{
			name:    "NoRings",
			polygon: NewPolygon([][]geom.Coord{}),
			want:    &pb.Polygon{Type: "Polygon"},
		},
		{
			name: "ExteriorRingOnly",
			polygon: NewPolygon([][]geom.Coord{
				{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
				// The hole must be dropped: pb.Polygon holds a single ring.
				{{1, 1}, {2, 1}, {2, 2}, {1, 2}, {1, 1}},
			}),
			want: &pb.Polygon{
				Type: "Polygon",
				Coordinates: []*pb.Coordinates{
					{Longitude: 0, Latitude: 0},
					{Longitude: 10, Latitude: 0},
					{Longitude: 10, Latitude: 10},
					{Longitude: 0, Latitude: 10},
					{Longitude: 0, Latitude: 0},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.polygon.ToProtobufObject())
		})
	}
}

func TestMultiPolygon_ToProtobufObject(t *testing.T) {
	var nilMultiPolygon *MultiPolygon
	assert.Nil(t, nilMultiPolygon.ToProtobufObject())
	assert.Nil(t, (&MultiPolygon{}).ToProtobufObject())

	mp := NewMultiPolygon([][][]geom.Coord{
		{{{0, 0}, {1, 0}, {1, 1}, {0, 0}}},
		{{{5, 5}, {6, 5}, {6, 6}, {5, 5}}, {{5.2, 5.2}, {5.4, 5.2}, {5.4, 5.4}, {5.2, 5.2}}},
	})

	got := mp.ToProtobufObject()
	require.Len(t, got, 2)
	assert.Equal(t, &pb.Polygon{
		Type: "Polygon",
		Coordinates: []*pb.Coordinates{
			{Longitude: 0, Latitude: 0},
			{Longitude: 1, Latitude: 0},
			{Longitude: 1, Latitude: 1},
			{Longitude: 0, Latitude: 0},
		},
	}, got[0])
	assert.Equal(t, &pb.Polygon{
		Type: "Polygon",
		Coordinates: []*pb.Coordinates{
			{Longitude: 5, Latitude: 5},
			{Longitude: 6, Latitude: 5},
			{Longitude: 6, Latitude: 6},
			{Longitude: 5, Latitude: 5},
		},
	}, got[1])
}
