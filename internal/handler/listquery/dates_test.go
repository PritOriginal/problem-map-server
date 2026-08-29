package listquery_test

import (
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/stretchr/testify/suite"
)

type DatesSuite struct {
	suite.Suite
}

func TestDatesSuite(t *testing.T) {
	suite.Run(t, new(DatesSuite))
}

func (suite *DatesSuite) TestParseDateRange() {
	day := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		from    string
		to      string
		want    models.DateRange
		wantErr string
	}{
		{name: "Empty"},
		{
			name: "RFC3339",
			from: "2025-03-01T12:00:00Z", to: "2025-03-02T12:00:00+03:00",
			want: models.DateRange{
				From: day.Add(12 * time.Hour),
				To:   time.Date(2025, 3, 2, 12, 0, 0, 0, time.FixedZone("", 3*3600)),
			},
		},
		{
			name: "RFC3339Nano",
			from: "2025-03-01T12:00:00.123456789Z",
			want: models.DateRange{From: day.Add(12*time.Hour + 123456789)},
		},
		{
			name: "DateOnly",
			from: "2025-03-01", to: "2025-03-01",
			want: models.DateRange{From: day, To: day.AddDate(0, 0, 1).Add(-time.Nanosecond)},
		},
		{
			name: "DateOnlyLeapDay",
			from: "2024-02-29", to: "2024-02-29",
			want: models.DateRange{
				From: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 2, 29, 23, 59, 59, 999999999, time.UTC),
			},
		},
		{name: "OnlyTo", to: "2025-03-01", want: models.DateRange{To: day.Add(24*time.Hour - time.Nanosecond)}},
		{name: "ErrFromInvalid", from: "yesterday", wantErr: "from must be RFC3339 or YYYY-MM-DD"},
		{name: "ErrFromNotADate", from: "2025-02-30", wantErr: "from must be RFC3339 or YYYY-MM-DD"},
		{name: "ErrToInvalid", from: "2025-03-01", to: "2025-03-01T25:00:00Z", wantErr: "to must be RFC3339 or YYYY-MM-DD"},
		{name: "ErrToDateTimeWithoutZone", to: "2025-03-01T12:00:00", wantErr: "to must be RFC3339 or YYYY-MM-DD"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			got, err := listquery.ParseDateRange(tt.from, tt.to)
			if tt.wantErr != "" {
				suite.EqualError(err, tt.wantErr)
				suite.Equal(models.DateRange{}, got)
				return
			}
			suite.NoError(err)
			suite.True(tt.want.From.Equal(got.From), "from: want %v, got %v", tt.want.From, got.From)
			suite.True(tt.want.To.Equal(got.To), "to: want %v, got %v", tt.want.To, got.To)
		})
	}
}

func (suite *DatesSuite) TestParseTime() {
	got, err := listquery.ParseTime("since", "2025-03-01")
	suite.NoError(err)
	suite.Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), got)

	got, err = listquery.ParseTimeEnd("created_to", "2025-03-01")
	suite.NoError(err)
	suite.Equal(time.Date(2025, 3, 1, 23, 59, 59, 999999999, time.UTC), got)

	got, err = listquery.ParseTime("since", "")
	suite.NoError(err)
	suite.True(got.IsZero())

	_, err = listquery.ParseTime("since", "01.03.2025")
	suite.EqualError(err, "since must be RFC3339 or YYYY-MM-DD")
}
