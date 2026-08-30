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

type PointJSON struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates" example:"0,0"`
}

func NewPoint(coords geom.Coord) *Point {
	return &Point{
		Ewkb: ewkb.Point{
			Point: geom.NewPoint(geom.XY).MustSetCoords(coords).SetSRID(4326),
		},
	}
}

// NewPointLonLat builds a WGS84 point from a longitude/latitude pair after
// ValidateLonLat, so an out-of-range or NaN coordinate never reaches the
// database.
func NewPointLonLat(lon, lat float64) (*Point, error) {
	if err := ValidateLonLat(lon, lat); err != nil {
		return nil, err
	}
	return NewPoint(geom.Coord{lon, lat}), nil
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
	if err := geojson.Unmarshal(data, &geometry); err != nil {
		return fmt.Errorf("unmarshal geometry: %w", err)
	}
	point, ok := geometry.(*geom.Point)
	if !ok {
		return fmt.Errorf("geometry is not a point")
	}
	ewkbPoint := ewkb.Point{Point: point}
	p.Ewkb = ewkbPoint

	return nil
}

// ToProtobufObject converts the point to its protobuf representation.
// A nil point yields nil.
func (p *Point) ToProtobufObject() *pb.Point {
	if p == nil || p.Ewkb.Point == nil {
		return nil
	}

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

type PolygonJSON struct {
	Type        string       `json:"type"`
	Coordinates [][2]float64 `json:"coordinates"`
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
	if err := geojson.Unmarshal(data, &geometry); err != nil {
		return fmt.Errorf("unmarshal geometry: %w", err)
	}
	polygon, ok := geometry.(*geom.Polygon)
	if !ok {
		return fmt.Errorf("geometry is not a polygon")
	}
	ewkbPolygon := ewkb.Polygon{Polygon: polygon}
	p.Ewkb = ewkbPolygon

	return nil
}

// ToProtobufObject converts the polygon to its protobuf representation.
// pb.Polygon carries a single ring, so only the exterior ring (holes are
// dropped) is exported. A nil polygon yields nil.
func (p *Polygon) ToProtobufObject() *pb.Polygon {
	if p == nil || p.Ewkb.Polygon == nil {
		return nil
	}

	return polygonToProtobufObject(p.Ewkb.Polygon)
}

func polygonToProtobufObject(polygon *geom.Polygon) *pb.Polygon {
	result := &pb.Polygon{Type: "Polygon"}
	if polygon.NumLinearRings() == 0 {
		return result
	}

	ring := polygon.LinearRing(0)
	result.Coordinates = make([]*pb.Coordinates, ring.NumCoords())
	for i, coord := range ring.Coords() {
		result.Coordinates[i] = &pb.Coordinates{
			Longitude: coord.X(),
			Latitude:  coord.Y(),
		}
	}

	return result
}

type MultiPolygon struct {
	Ewkb ewkb.MultiPolygon
}

type MultiPolygonJSON struct {
	Type        string           `json:"type"`
	Coordinates [][][][2]float64 `json:"coordinates"`
}

func NewMultiPolygon(coords [][][]geom.Coord) *MultiPolygon {
	return &MultiPolygon{
		Ewkb: ewkb.MultiPolygon{
			MultiPolygon: geom.NewMultiPolygon(geom.XY).MustSetCoords(coords).SetSRID(4326),
		},
	}
}

func (p *MultiPolygon) Scan(src interface{}) error {
	return p.Ewkb.Scan(src)
}

func (p *MultiPolygon) Valid() bool {
	return p.Ewkb.Valid()
}

func (p *MultiPolygon) Value() (driver.Value, error) {
	return p.Ewkb.Value()
}

func (p *MultiPolygon) MarshalJSON() ([]byte, error) {
	geometry, err := geojson.Marshal(p.Ewkb.MultiPolygon)
	if err != nil {
		return []byte{}, err
	}

	return geometry, nil
}

// ToProtobufObject converts the multipolygon to a list of protobuf polygons
// (there is no MultiPolygon message in the protos); each element carries the
// exterior ring of the corresponding polygon. A nil multipolygon yields nil.
func (p *MultiPolygon) ToProtobufObject() []*pb.Polygon {
	if p == nil || p.Ewkb.MultiPolygon == nil {
		return nil
	}

	result := make([]*pb.Polygon, p.Ewkb.NumPolygons())
	for i := range result {
		result[i] = polygonToProtobufObject(p.Ewkb.Polygon(i))
	}

	return result
}

func (p *MultiPolygon) UnmarshalJSON(data []byte) error {
	var geometry geom.T
	if err := geojson.Unmarshal(data, &geometry); err != nil {
		return fmt.Errorf("unmarshal geometry: %w", err)
	}
	multiPolygon, ok := geometry.(*geom.MultiPolygon)
	if !ok {
		return fmt.Errorf("geometry is not a multi polygon")
	}
	ewkbMultiPolygon := ewkb.MultiPolygon{MultiPolygon: multiPolygon}
	p.Ewkb = ewkbMultiPolygon

	return nil
}
