package models

import (
	"encoding/json"

	"github.com/twpayne/go-geom/encoding/ewkb"
	"github.com/twpayne/go-geom/encoding/geojson"
)

// type Geom struct {
// 	Type string
// 	Coordinates
// }

type Region struct {
	ID   int          `json:"region_id" db:"region_id"`
	Name string       `json:"name"`
	Geom ewkb.Polygon `json:"geom"`
}

type City struct {
	ID       int          `json:"city_id" db:"city_id"`
	Name     string       `json:"name"`
	RegionID int          `json:"region_id" db:"region_id"`
	Geom     ewkb.Polygon `json:"geom"`
}

func (d *City) MarshalJSON() ([]byte, error) {
	geometry, err := geojson.Marshal(d.Geom.Polygon)
	if err != nil {
		return []byte{}, err
	}

	type ALias City
	return json.Marshal(&struct {
		Geom json.RawMessage `json:"geom"`
		*ALias
	}{
		Geom:  geometry,
		ALias: (*ALias)(d),
	})
}

type District struct {
	ID   int          `json:"district_id" db:"district_id"`
	Name string       `json:"name"`
	Geom ewkb.Polygon `json:"geom"`
}

func (d *District) MarshalJSON() ([]byte, error) {
	geometry, err := geojson.Marshal(d.Geom.Polygon)
	if err != nil {
		return []byte{}, err
	}

	type ALias District
	return json.Marshal(&struct {
		Geom json.RawMessage `json:"geom"`
		*ALias
	}{
		Geom:  geometry,
		ALias: (*ALias)(d),
	})
}

type Mark struct {
	ID           int        `json:"mark_id" db:"mark_id"`
	Name         string     `json:"name"`
	Geom         ewkb.Point `json:"geom"`
	TypeMarkID   int        `json:"type_mark_id" db:"type_mark_id"`
	UserID       int        `json:"user_id" db:"user_id"`
	DistrictID   int        `json:"district_id" db:"district_id"`
	NumberVotes  int        `json:"number_votes" db:"number_votes"`
	NumberChecks int        `json:"number_checks" db:"number_checks"`
}

func (d *Mark) MarshalJSON() ([]byte, error) {
	geometry, err := geojson.Marshal(d.Geom.Point)
	if err != nil {
		return []byte{}, err
	}

	type ALias Mark
	return json.Marshal(&struct {
		Geom json.RawMessage `json:"geom"`
		*ALias
	}{
		Geom:  geometry,
		ALias: (*ALias)(d),
	})
}

type TypeMark struct {
	ID   int    `json:"type_mark_id" db:"type_mark_id"`
	Name string `json:"name"`
}

type User struct {
	ID     int    `json:"user_id" db:"user_id"`
	Name   string `json:"name"`
	Rating int    `json:"rating"`
}

type Task struct {
	ID     int    `json:"task_id" db:"task_id"`
	Name   string `json:"name"`
	UserID int    `json:"user_id" db:"user_id"`
}
