package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SLASuite struct {
	suite.Suite
	uc        *usecase.SLA
	marks     *usecase.MockSLAMarksRepository
	publisher *events.MockPublisher
}

func (suite *SLASuite) SetupTest() {
	suite.marks = usecase.NewMockSLAMarksRepository(suite.T())
	suite.publisher = events.NewMockPublisher(suite.T())
	suite.uc = usecase.NewSLA(slogdiscard.NewDiscardLogger(), usecase.SLARepositories{Marks: suite.marks}).WithEvents(suite.publisher)
}

func TestSLA(t *testing.T) {
	suite.Run(t, new(SLASuite))
}

func (suite *SLASuite) TestExpireOverdue() {
	dueAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	overdue := []models.Mark{
		{ID: 1, OrganizationID: null.IntFrom(10), SLADueAt: null.TimeFrom(dueAt)},
		{ID: 2, OrganizationID: null.IntFrom(11), SLADueAt: null.TimeFrom(dueAt.Add(time.Hour))},
	}

	tests := []struct {
		name    string
		marks   method[[]models.Mark]
		want    int
		wantErr bool
	}{
		{name: "Ok", marks: method[[]models.Mark]{data: overdue}, want: 2},
		{name: "Empty", marks: method[[]models.Mark]{data: []models.Mark{}}, want: 0},
		{name: "ErrRepo", marks: method[[]models.Mark]{err: errors.New("db")}, wantErr: true},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			suite.marks.On("GetOverdueMarks", mock.Anything, mock.AnythingOfType("time.Time")).Once().Return(tt.marks.data, tt.marks.err)
			var ids []string
			for _, m := range tt.marks.data {
				suite.publisher.On("Publish", mock.Anything, events.SubjectMarkSLABreached, mock.MatchedBy(func(ev events.MarkSLABreached) bool {
					return ev.MarkID == m.ID && ev.OrganizationID == int(m.OrganizationID.Int64)
				})).Once().Run(func(args mock.Arguments) {
					ids = append(ids, args.Get(2).(events.MarkSLABreached).EventID)
				}).Return(nil)
			}

			n, err := suite.uc.ExpireOverdue(context.Background())
			if tt.wantErr {
				suite.Error(err)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(tt.want, n)

			// The same deadline yields the same event id on every run.
			for i, m := range tt.marks.data {
				suite.Equal(events.NewMarkSLABreached(m.ID, int(m.OrganizationID.Int64), m.SLADueAt.Time).EventID, ids[i])
			}
		})
	}
}
