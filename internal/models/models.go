package models

import (
	"time"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
)

type Region struct {
	ID   int      `json:"region_id" db:"region_id"`
	Name string   `json:"name"`
	Geom *Polygon `json:"geom"`
}

func (r *Region) ToProtobufObject() *pb.Region {
	return &pb.Region{
		Id:   int64(r.ID),
		Name: r.Name,
		Geom: r.Geom.ToProtobufObject(),
	}
}

type City struct {
	ID       int      `json:"city_id" db:"city_id"`
	Name     string   `json:"name"`
	RegionID int      `json:"region_id" db:"region_id"`
	Geom     *Polygon `json:"geom"`
}

func (c *City) ToProtobufObject() *pb.City {
	return &pb.City{
		Id:       int64(c.ID),
		Name:     c.Name,
		RegionId: int64(c.RegionID),
		Geom:     c.Geom.ToProtobufObject(),
	}
}

type District struct {
	ID     int      `json:"district_id" db:"district_id"`
	Name   string   `json:"name"`
	CityID int      `json:"city_id"`
	Geom   *Polygon `json:"geom"`
}

func (d *District) ToProtobufObject() *pb.District {
	return &pb.District{
		Id:     int64(d.ID),
		Name:   d.Name,
		CityId: int64(d.CityID),
		Geom:   d.Geom.ToProtobufObject(),
	}
}

type Mark struct {
	ID           int       `json:"mark_id" db:"mark_id"`
	Description  string    `json:"description" db:"description"`
	Geom         *Point    `json:"geom" db:"geom"`
	MarkTypeID   int       `json:"mark_type_id" db:"type_mark_id"`
	MarkStatusID int       `json:"mark_status_id" db:"mark_status_id"`
	UserID       int       `json:"user_id" db:"user_id"`
	NumberVotes  int       `json:"number_votes" db:"number_votes"`
	NumberChecks int       `json:"number_checks" db:"number_checks"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

func (m *Mark) ToProtobufObject() *pb.Mark {
	return &pb.Mark{
		Id:           int64(m.ID),
		Name:         m.Name,
		Geom:         m.Geom.ToProtobufObject(),
		TypeMarkId:   int64(m.TypeMarkID),
		UserId:       int64(m.UserID),
		DistrictId:   int64(m.DistrictID),
		NumberVotes:  int64(m.NumberVotes),
		NumberChecks: int64(m.NumberChecks),
	}
}

type MarkType struct {
	ID   int    `json:"type_mark_id" db:"type_mark_id"`
	Name string `json:"name"`
}

type MarkStatus struct {
	ID   int    `json:"mark_status_id" db:"mark_status_id"`
	Name string `json:"name" db:"name"`
}

type Check struct {
	ID        int       `json:"check_id" db:"check_id"`
	UserID    int       `json:"user_id" db:"user_id"`
	MarkID    int       `json:"mark_id" db:"mark_id"`
	Result    bool      `json:"result" db:"result"`
	Comment   string    `json:"comment" db:"comment"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
