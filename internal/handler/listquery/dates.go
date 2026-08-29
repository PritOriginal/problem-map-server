package listquery

import (
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

// DateFormatsHint is the human-readable list of accepted timestamp formats,
// shared by the parsing errors and the swagger descriptions.
const DateFormatsHint = "RFC3339 or YYYY-MM-DD"

// ParseTime parses the timestamp query parameter with the given name.
// It accepts RFC3339 (with or without fractional seconds) and a bare
// YYYY-MM-DD date, which is taken as the start of that day in UTC.
// An empty value yields the zero time. Returned errors are safe to show
// to the client.
func ParseTime(name, value string) (time.Time, error) {
	return parseTime(name, value, false)
}

// ParseTimeEnd is ParseTime for an upper bound: a bare YYYY-MM-DD date is
// taken as the last nanosecond of that day in UTC, so the whole day is
// included in an inclusive range.
func ParseTimeEnd(name, value string) (time.Time, error) {
	return parseTime(name, value, true)
}

func parseTime(name, value string, endOfDay bool) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be %s", name, DateFormatsHint)
	}
	if endOfDay {
		t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return t, nil
}

// ParseDateRange parses the optional ?from=&to= bounds of a period with
// ParseTime / ParseTimeEnd; empty values leave the bound open.
func ParseDateRange(from, to string) (models.DateRange, error) {
	var r models.DateRange
	var err error
	if r.From, err = ParseTime("from", from); err != nil {
		return models.DateRange{}, err
	}
	if r.To, err = ParseTimeEnd("to", to); err != nil {
		return models.DateRange{}, err
	}
	return r, nil
}
