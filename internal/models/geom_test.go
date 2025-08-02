package models

import (
	"reflect"
	"testing"

	"github.com/twpayne/go-geom"
)

func TestPoint_UnmarshalJSON(t *testing.T) {
	expectedPoint := NewPoint(geom.Coord{41.463077, 52.718319})
	p := &Point{}
	data := []byte(`{"type":"Point", "coordinates": [41.463077,52.718319]}`)
	if err := p.UnmarshalJSON(data); err != nil {
		t.Errorf("Point.UnmarshalJSON() error = %v", err)
	}
	p.Ewkb.Point.SetSRID(4326)
	if !reflect.DeepEqual(p, expectedPoint) {
		t.Errorf("Points not equal")
	}
}
