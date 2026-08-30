package models

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// DefaultLimit is the page size used when a client does not pass limit.
	DefaultLimit = 100
	// MaxLimit is the largest page size a client may request.
	MaxLimit = 500
)

var (
	ErrInvalidPagination = errors.New("invalid pagination")
	ErrInvalidBBox       = errors.New("invalid bbox")
	ErrInvalidSort       = errors.New("invalid sort")
)

// Pagination describes a limit/offset window over a list query.
// The zero value means "no limit" and is what internal callers
// (tasker, gRPC) use to fetch everything; REST handlers always set a
// limit (see handler/listquery) so that the public API paginates.
type Pagination struct {
	Limit  int
	Offset int
}

// Validate reports an error when limit or offset are out of range.
// Limit 0 is allowed and means "not set".
func (p Pagination) Validate() error {
	if p.Limit < 0 || p.Limit > MaxLimit {
		return fmt.Errorf("%w: limit must be at most %d", ErrInvalidPagination, MaxLimit)
	}
	if p.Offset < 0 {
		return fmt.Errorf("%w: offset must be non-negative", ErrInvalidPagination)
	}
	return nil
}

// Page is a window of items together with the total number of matching rows.
type Page[T any] struct {
	Items []T
	Total int
}

// BBox is a WGS84 bounding box: minLon,minLat,maxLon,maxLat.
type BBox struct {
	MinLon float64
	MinLat float64
	MaxLon float64
	MaxLat float64
}

// ParseBBox parses "minLon,minLat,maxLon,maxLat" and validates it.
func ParseBBox(s string) (BBox, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return BBox{}, fmt.Errorf("%w: expected 4 comma-separated numbers", ErrInvalidBBox)
	}
	var vals [4]float64
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return BBox{}, fmt.Errorf("%w: %q is not a number", ErrInvalidBBox, part)
		}
		vals[i] = v
	}
	b := BBox{MinLon: vals[0], MinLat: vals[1], MaxLon: vals[2], MaxLat: vals[3]}
	if err := b.Validate(); err != nil {
		return BBox{}, err
	}
	return b, nil
}

// ErrInvalidCoordinates is returned for a lon/lat pair outside WGS84 ranges.
var ErrInvalidCoordinates = errors.New("invalid coordinates")

// ValidateLonLat checks that a WGS84 pair is finite and in range. NaN and
// Inf are rejected explicitly because NaN slips through range comparisons.
func ValidateLonLat(lon, lat float64) error {
	if math.IsNaN(lon) || math.IsInf(lon, 0) || math.IsNaN(lat) || math.IsInf(lat, 0) {
		return fmt.Errorf("%w: coordinates must be finite numbers", ErrInvalidCoordinates)
	}
	if lon < -180 || lon > 180 {
		return fmt.Errorf("%w: longitude must be between -180 and 180", ErrInvalidCoordinates)
	}
	if lat < -90 || lat > 90 {
		return fmt.Errorf("%w: latitude must be between -90 and 90", ErrInvalidCoordinates)
	}
	return nil
}

// Validate checks coordinate ranges and that min < max on both axes.
func (b BBox) Validate() error {
	if err := ValidateLonLat(b.MinLon, b.MinLat); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBBox, err)
	}
	if err := ValidateLonLat(b.MaxLon, b.MaxLat); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBBox, err)
	}
	if b.MinLon >= b.MaxLon || b.MinLat >= b.MaxLat {
		return fmt.Errorf("%w: min must be less than max", ErrInvalidBBox)
	}
	return nil
}

// SortOrder is the direction of an ORDER BY clause.
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// Validate accepts the empty value (caller applies its default).
func (o SortOrder) Validate() error {
	switch o {
	case "", SortAsc, SortDesc:
		return nil
	default:
		return fmt.Errorf("%w: order must be asc or desc", ErrInvalidSort)
	}
}

// MarksSort is the column marks lists can be sorted by.
type MarksSort string

const (
	MarksSortCreatedAt MarksSort = "created_at"
	MarksSortUpdatedAt MarksSort = "updated_at"
)

// Validate accepts the empty value (caller applies its default).
func (s MarksSort) Validate() error {
	switch s {
	case "", MarksSortCreatedAt, MarksSortUpdatedAt:
		return nil
	default:
		return fmt.Errorf("%w: sort must be created_at or updated_at", ErrInvalidSort)
	}
}
