package models

import (
	"database/sql/driver"
	"fmt"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
	"github.com/twpayne/go-geom/encoding/geojson"
)

type Point struct {
	Ewkb ewkb.Point
}

func NewPoint(coords geom.Coord) *Point {
	return &Point{
		Ewkb: ewkb.Point{
			Point: geom.NewPoint(geom.XY).MustSetCoords(coords).SetSRID(4326),
		},
	}
}

func (p *Point) Scan(src interface{}) error {
	return p.Ewkb.Scan(src)
}

func (p *Point) Valid() bool {
	return p.Ewkb.Valid()
}

func (p *Point) Value() (driver.Value, error) {
	return p.Ewkb.Value()
}

func (p *Point) MarshalJSON() ([]byte, error) {
	geometry, err := geojson.Marshal(p.Ewkb.Point)
	if err != nil {
		return []byte{}, err
	}

	return geometry, nil
}

func (p *Point) UnmarshalJSON(data []byte) error {
	var geometry geom.T
	geojson.Unmarshal(data, &geometry)
	point, ok := geometry.(*geom.Point)
	if !ok {
		return fmt.Errorf("geometry is not a point")
	}
	ewkbPoint := ewkb.Point{Point: point}
	p.Ewkb = ewkbPoint

	return nil
}

func (p *Point) ToProtobufObject() *pb.Point {
	return &pb.Point{
		Type: "Point",
		Coordinates: &pb.Coordinates{
			Longitude: p.Ewkb.Coords().X(),
			Latitude:  p.Ewkb.Coords().Y(),
		},
	}
}

type Polygon struct {
	Ewkb ewkb.Polygon
}

func NewPolygon(coords [][]geom.Coord) *Polygon {
	return &Polygon{
		Ewkb: ewkb.Polygon{
			Polygon: geom.NewPolygon(geom.XY).MustSetCoords(coords).SetSRID(4326),
		},
	}
}

func (p *Polygon) Scan(src interface{}) error {
	return p.Ewkb.Scan(src)
}

func (p *Polygon) Valid() bool {
	return p.Ewkb.Valid()
}

func (p *Polygon) Value() (driver.Value, error) {
	return p.Ewkb.Value()
}

func (p *Polygon) MarshalJSON() ([]byte, error) {
	geometry, err := geojson.Marshal(p.Ewkb.Polygon)
	if err != nil {
		return []byte{}, err
	}

	return geometry, nil
}

func (p *Polygon) UnmarshalJSON(data []byte) error {
	var geometry geom.T
	geojson.Unmarshal(data, &geometry)
	polygon, ok := geometry.(*geom.Polygon)
	if !ok {
		return fmt.Errorf("geometry is not a point")
	}
	ewkbPolygon := ewkb.Polygon{Polygon: polygon}
	p.Ewkb = ewkbPolygon

	return nil
}

func (p *Polygon) ToProtobufObject() *pb.Polygon {
	p.Ewkb.Polygon.Coords()
	return &pb.Polygon{
		Type: "Polygon",
		// TODO: Coordinates
	}
}
