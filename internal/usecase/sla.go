package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
)

// SLAMarksRepository lists the assigned marks whose deadline has passed and
// records that the breach was reported.
type SLAMarksRepository interface {
	// GetOverdueMarks returns the overdue marks whose breach has not been
	// reported yet (sla_breached_at IS NULL).
	GetOverdueMarks(ctx context.Context, now time.Time) ([]models.Mark, error)
	// MarkSLABreached records that the breach of the deadline dueAt was
	// reported; a mark whose deadline changed meanwhile is left untouched.
	MarkSLABreached(ctx context.Context, markId int, dueAt time.Time) error
}

type SLARepositories struct {
	Marks SLAMarksRepository
}

// SLA is the periodic check of organization deadlines (run by cmd/tasker).
// Overdue marks keep their status; is_overdue is computed on read. The
// check publishes mark.sla_breached once per breach: a mark is stamped
// (sla_breached_at) after its event was published and is not listed
// again until its deadline is reset by a (re)assignment. A failed publish
// leaves the mark unstamped, so it is retried on the next run; the event id
// is derived from (mark_id, sla_due_at), so such a retry never produces a
// duplicate notification.
type SLA struct {
	log    *slog.Logger
	repos  SLARepositories
	events events.Publisher
	now    func() time.Time
}

func NewSLA(log *slog.Logger, repos SLARepositories) *SLA {
	return &SLA{
		log:    log,
		repos:  repos,
		events: events.NoopPublisher{},
		now:    time.Now,
	}
}

// WithEvents sets the publisher of mark.sla_breached events. Without it
// events are dropped.
func (uc *SLA) WithEvents(p events.Publisher) *SLA {
	if p != nil {
		uc.events = p
	}
	return uc
}

// ExpireOverdue publishes mark.sla_breached for every assigned mark whose
// deadline has passed since the previous run and returns the number of
// breaches reported.
func (uc *SLA) ExpireOverdue(ctx context.Context) (int, error) {
	const op = "usecase.SLA.ExpireOverdue"

	marks, err := uc.repos.Marks.GetOverdueMarks(ctx, uc.now())
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	reported := 0
	for _, mark := range marks {
		if !mark.OrganizationID.Valid || !mark.SLADueAt.Valid {
			continue
		}
		ev := events.NewMarkSLABreached(mark.ID, int(mark.OrganizationID.Int64), mark.SLADueAt.Time)
		if err := uc.events.Publish(ctx, ev.Subject(), ev); err != nil {
			// Not stamped: the breach is reported again on the next run.
			uc.log.Warn("failed to publish sla breach, will retry",
				slog.String("op", op), slog.Int("mark_id", mark.ID), logger.Err(err))
			continue
		}
		if err := uc.repos.Marks.MarkSLABreached(ctx, mark.ID, mark.SLADueAt.Time); err != nil {
			return reported, fmt.Errorf("%s: %w", op, err)
		}
		reported++
	}
	uc.log.Info("sla check finished", slog.String("op", op), slog.Int("overdue", len(marks)), slog.Int("reported", reported))

	return reported, nil
}
