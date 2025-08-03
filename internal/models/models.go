package models

type Region struct {
	ID   int     `json:"region_id" db:"region_id"`
	Name string  `json:"name"`
	Geom Polygon `json:"geom"`
}

type City struct {
	ID       int     `json:"city_id" db:"city_id"`
	Name     string  `json:"name"`
	RegionID int     `json:"region_id" db:"region_id"`
	Geom     Polygon `json:"geom"`
}

type District struct {
	ID   int     `json:"district_id" db:"district_id"`
	Name string  `json:"name"`
	Geom Polygon `json:"geom"`
}

type Mark struct {
	ID           int    `json:"mark_id" db:"mark_id"`
	Name         string `json:"name"`
	Geom         Point  `json:"geom"`
	TypeMarkID   int    `json:"type_mark_id" db:"type_mark_id"`
	UserID       int    `json:"user_id" db:"user_id"`
	DistrictID   int    `json:"district_id" db:"district_id"`
	NumberVotes  int    `json:"number_votes" db:"number_votes"`
	NumberChecks int    `json:"number_checks" db:"number_checks"`
}

type TypeMark struct {
	ID   int    `json:"type_mark_id" db:"type_mark_id"`
	Name string `json:"name"`
}
