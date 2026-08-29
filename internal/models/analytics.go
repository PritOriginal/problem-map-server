package models

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/guregu/null/v6"
)

// ErrInvalidDateRange is returned when "to" is before "from".
var ErrInvalidDateRange = errors.New("invalid date range")

// DateRange bounds a period (inclusive); a zero bound means unbounded.
type DateRange struct {
	From time.Time
	To   time.Time
}

// Validate rejects a range whose end precedes its start.
func (r DateRange) Validate() error {
	if !r.From.IsZero() && !r.To.IsZero() && r.To.Before(r.From) {
		return fmt.Errorf("%w: to is before from", ErrInvalidDateRange)
	}
	return nil
}

// AnalyticsFilters narrows the set of marks analytics are computed over.
// BoundaryID restricts marks to those inside the admin boundary
// (ST_Contains), MarkTypeID to a single type; zero means no restriction.
// The date range bounds marks.created_at.
type AnalyticsFilters struct {
	BoundaryID int
	MarkTypeID int
	DateRange
}

// Validate checks ids and the date range.
func (f AnalyticsFilters) Validate() error {
	if f.BoundaryID < 0 {
		return errors.New("boundary_id must be positive")
	}
	if f.MarkTypeID < 0 {
		return errors.New("mark_type_id must be positive")
	}
	return f.DateRange.Validate()
}

// KPI is the summary of marks matching AnalyticsFilters. Durations are
// derived from mark_status_history: the gap between the first "unconfirmed"
// record of a mark and its first "confirmed" / "closed" record. A null
// duration means no mark reached that status.
type KPI struct {
	Total    int         `json:"total"`
	ByStatus map[int]int `json:"by_status"`
	// AvgConfirmHours / MedianConfirmHours: unconfirmed -> confirmed.
	AvgConfirmHours    null.Float `json:"avg_confirm_hours" swaggertype:"number"`
	MedianConfirmHours null.Float `json:"median_confirm_hours" swaggertype:"number"`
	// AvgCloseHours: unconfirmed -> closed.
	AvgCloseHours null.Float `json:"avg_close_hours" swaggertype:"number"`
	// RefutedShare is refuted marks / total (0 when there are no marks).
	RefutedShare float64 `json:"refuted_share"`
	// OpenOlderThan30d counts marks neither closed nor refuted that were
	// created more than 30 days ago.
	OpenOlderThan30d int `json:"open_older_than_30d"`
}

// TimeseriesStep is the bucket size of a time series.
type TimeseriesStep string

const (
	StepDay   TimeseriesStep = "day"
	StepWeek  TimeseriesStep = "week"
	StepMonth TimeseriesStep = "month"
)

// Validate accepts only the known steps; the empty value is rejected so the
// caller applies its default explicitly.
func (s TimeseriesStep) Validate() error {
	switch s {
	case StepDay, StepWeek, StepMonth:
		return nil
	default:
		return fmt.Errorf("invalid step %q", string(s))
	}
}

// Duration returns the nominal length of one bucket, used to size default
// ranges and to bound the number of buckets.
func (s TimeseriesStep) Duration() time.Duration {
	switch s {
	case StepWeek:
		return 7 * 24 * time.Hour
	case StepMonth:
		return 31 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// MaxTimeseriesPeriods caps the number of buckets in one response.
const MaxTimeseriesPeriods = 1000

// TimeseriesFilters selects the marks and the bucketing of a time series.
// Both range bounds must be set (the usecase applies defaults).
type TimeseriesFilters struct {
	AnalyticsFilters
	Step TimeseriesStep
}

// Validate checks the filters, the step and the number of buckets.
func (f TimeseriesFilters) Validate() error {
	if err := f.AnalyticsFilters.Validate(); err != nil {
		return err
	}
	if err := f.Step.Validate(); err != nil {
		return err
	}
	if f.From.IsZero() || f.To.IsZero() {
		return errors.New("from and to are required")
	}
	if periods := f.To.Sub(f.From) / f.Step.Duration(); periods > MaxTimeseriesPeriods {
		return fmt.Errorf("too many periods (%d > %d)", periods, MaxTimeseriesPeriods)
	}
	return nil
}

// TimeseriesPoint holds the events that happened in one bucket: marks
// created and status transitions to confirmed / closed / refuted.
type TimeseriesPoint struct {
	Period    time.Time `json:"period" db:"period"`
	Created   int       `json:"created" db:"created"`
	Confirmed int       `json:"confirmed" db:"confirmed"`
	Closed    int       `json:"closed" db:"closed"`
	Refuted   int       `json:"refuted" db:"refuted"`
}

const (
	DefaultTopTypesLimit = 10
	MaxTopTypesLimit     = 100
)

// TopTypesFilters selects the marks ranked by type.
type TopTypesFilters struct {
	BoundaryID int
	DateRange
	// Limit caps the number of rows; zero means DefaultTopTypesLimit.
	Limit int
}

// Validate checks the boundary, the date range and the limit.
func (f TopTypesFilters) Validate() error {
	if f.BoundaryID < 0 {
		return errors.New("boundary_id must be positive")
	}
	if err := f.DateRange.Validate(); err != nil {
		return err
	}
	if f.Limit < 0 || f.Limit > MaxTopTypesLimit {
		return fmt.Errorf("limit must be in 0..%d", MaxTopTypesLimit)
	}
	return nil
}

// TopType is one row of the mark-type ranking; Share is Count over the
// total number of matching marks.
type TopType struct {
	MarkTypeID int     `json:"mark_type_id" db:"mark_type_id"`
	Name       string  `json:"name" db:"name"`
	Count      int     `json:"count" db:"count"`
	Share      float64 `json:"share" db:"share"`
}

const (
	DefaultHeatmapCellM = 250
	MinHeatmapCellM     = 10
	MaxHeatmapCellM     = 100_000
	// MaxHeatmapCells caps the number of hexagons a heatmap may contain.
	MaxHeatmapCells = 5000
)

// ErrTooManyHeatmapCells is returned when the bbox / cell size combination
// would produce more than MaxHeatmapCells hexagons.
var ErrTooManyHeatmapCells = errors.New("too many heatmap cells, increase cell_m")

// HeatmapFilters selects the marks binned into a hexagonal grid. CellM is the
// hexagon size in meters (center-to-vertex, EPSG:3857).
type HeatmapFilters struct {
	BBox          BBox
	CellM         float64
	MarkTypeIds   []int
	MarkStatusIds []int
}

// Validate checks the bbox, the cell size and the estimated cell count.
func (f HeatmapFilters) Validate() error {
	if err := f.BBox.Validate(); err != nil {
		return err
	}
	if f.CellM < MinHeatmapCellM || f.CellM > MaxHeatmapCellM {
		return fmt.Errorf("cell_m must be in %d..%d", MinHeatmapCellM, MaxHeatmapCellM)
	}
	if f.EstimateCells() > MaxHeatmapCells {
		return ErrTooManyHeatmapCells
	}
	return nil
}

// EstimateCells approximates how many hexagons of CellM cover the bbox in
// EPSG:3857: the grid has a column every 1.5*size meters and a row every
// sqrt(3)*size meters, plus one partial column and row along the edges.
func (f HeatmapFilters) EstimateCells() int {
	minX, minY := webMercator(f.BBox.MinLon, f.BBox.MinLat)
	maxX, maxY := webMercator(f.BBox.MaxLon, f.BBox.MaxLat)
	width, height := maxX-minX, maxY-minY

	cols := math.Ceil(width/(1.5*f.CellM)) + 1
	rows := math.Ceil(height/(math.Sqrt(3)*f.CellM)) + 1
	return int(cols * rows)
}

// webMercator projects a WGS84 coordinate to EPSG:3857 meters; latitudes are
// clamped to the projection's valid range.
func webMercator(lon, lat float64) (x, y float64) {
	const earthRadius = 6378137.0
	const maxLat = 85.05112878
	lat = math.Max(-maxLat, math.Min(maxLat, lat))
	x = earthRadius * lon * math.Pi / 180
	y = earthRadius * math.Log(math.Tan(math.Pi/4+lat*math.Pi/360))
	return x, y
}

// HeatmapCell is one hexagon of the grid with the number of marks inside.
type HeatmapCell struct {
	Geom  *Polygon `json:"geom" db:"geom"`
	Count int      `json:"count" db:"count"`
}
