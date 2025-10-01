package models

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/twpayne/go-geom"
)

func TestPoint_UnmarshalJSON(t *testing.T) {
	expectedPoint := NewPoint(geom.Coord{41.463077, 52.718319})

	p := &Point{}
	data := []byte(`{"type":"Point","coordinates":[41.463077,52.718319]}`)
	if err := p.UnmarshalJSON(data); err != nil {
		t.Errorf("Point.UnmarshalJSON() error = %v", err)
	}
	p.Ewkb.Point.SetSRID(4326)
	if !reflect.DeepEqual(p, expectedPoint) {
		t.Errorf("Points not equal")
	}
}

func TestPoint_MarshalJSON(t *testing.T) {
	expectedPointJSON := []byte(`{"type":"Point","coordinates":[41.463077,52.718319]}`)

	p := NewPoint(geom.Coord{41.463077, 52.718319})
	pointJSON, err := p.MarshalJSON()
	if err != nil {
		t.Errorf("Point.MarshalJSON() error = %v", err)
	}
	if !reflect.DeepEqual(pointJSON, expectedPointJSON) {
		t.Errorf("Points not equal")
	}
}

func TestPolygon_UnmarshalJSON(t *testing.T) {
	expectedPolygon := NewPolygon([][]geom.Coord{
		{
			{41.462560, 52.718741},
			{41.463432, 52.717594},
			{41.462969, 52.717461},
			{41.462824, 52.717618},
			{41.462963, 52.717666},
			{41.462227, 52.718649},
		},
	})

	p := &Polygon{}
	data := []byte(`{"type":"Polygon","coordinates":[[[41.462560,52.718741],[41.463432,52.717594],[41.462969,52.717461],[41.462824,52.717618],[41.462963,52.717666],[41.462227,52.718649]]]}`)
	if err := p.UnmarshalJSON(data); err != nil {
		t.Errorf("Polygon.UnmarshalJSON() error = %v", err)
	}
	p.Ewkb.Polygon.SetSRID(4326)
	if !reflect.DeepEqual(p, expectedPolygon) {
		t.Errorf("Polygons not equal")
	}
}

func TestPolygon_MarshalJSON(t *testing.T) {
	expectedPolygonJSON := []byte(`{"type":"Polygon","coordinates":[[[41.46256,52.718741],[41.463432,52.717594],[41.462969,52.717461],[41.462824,52.717618],[41.462963,52.717666],[41.462227,52.718649]]]}`)

	p := NewPolygon([][]geom.Coord{
		{
			{41.462560, 52.718741},
			{41.463432, 52.717594},
			{41.462969, 52.717461},
			{41.462824, 52.717618},
			{41.462963, 52.717666},
			{41.462227, 52.718649},
		},
	})
	polygonJSON, err := p.MarshalJSON()
	if err != nil {
		t.Errorf("Polygon.MarshalJSON() error = %v", err)
	}
	fmt.Printf("%s\n", string(polygonJSON))
	if !reflect.DeepEqual(polygonJSON, expectedPolygonJSON) {
		t.Errorf("Polygons not equal")
	}
}
