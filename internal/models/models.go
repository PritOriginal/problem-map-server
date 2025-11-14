package models

import pb "github.com/PritOriginal/problem-map-protos/gen/go"

type Region struct {
	ID   int      `json:"region_id" db:"region_id"`
	Name string   `json:"name"`
	Geom *Polygon `json:"geom"`
}

func (r *Region) MarshalProtobuf() *pb.Region {
	return &pb.Region{
		Id:   int64(r.ID),
		Name: r.Name,
		Geom: r.Geom.MarshalProtobuf(),
	}
}

type City struct {
	ID       int      `json:"city_id" db:"city_id"`
	Name     string   `json:"name"`
	RegionID int      `json:"region_id" db:"region_id"`
	Geom     *Polygon `json:"geom"`
}

func (c *City) MarshalProtobuf() *pb.City {
	return &pb.City{
		Id:       int64(c.ID),
		Name:     c.Name,
		RegionId: int64(c.RegionID),
		Geom:     c.Geom.MarshalProtobuf(),
	}
}

type District struct {
	ID     int      `json:"district_id" db:"district_id"`
	Name   string   `json:"name"`
	CityID int      `json:"city_id"`
	Geom   *Polygon `json:"geom"`
}

func (m *District) MarshalProtobuf() *pb.District {
	return &pb.District{
		Id:     int64(m.ID),
		Name:   m.Name,
		CityId: int64(m.CityID),
		Geom:   m.Geom.MarshalProtobuf(),
	}
}

type Mark struct {
	ID           int    `json:"mark_id" db:"mark_id"`
	Name         string `json:"name"`
	Geom         *Point `json:"geom"`
	TypeMarkID   int    `json:"type_mark_id" db:"type_mark_id"`
	MarkStatusID int    `json:"mark_status_id" db:"mark_status_id"`
	UserID       int    `json:"user_id" db:"user_id"`
	DistrictID   int    `json:"district_id" db:"district_id"`
	NumberVotes  int    `json:"number_votes" db:"number_votes"`
	NumberChecks int    `json:"number_checks" db:"number_checks"`
}

func (m *Mark) MarshalProtobuf() *pb.Mark {
	return &pb.Mark{
		Id:           int64(m.ID),
		Name:         m.Name,
		Geom:         m.Geom.MarshalProtobuf(),
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

type StatusMark struct {
	ID   int    `json:"mark_status_id" db:"mark_status_id"`
	Nmae string `json:"name" db:"name"`
}
