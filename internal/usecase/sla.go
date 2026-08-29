package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// SLAMarksRepository lists the assigned marks whose deadline has passed.
type SLAMarksRepository interface {
	GetOverdueMarks(ctx context.Context, now time.Time) ([]models.Mark, error)
}

type SLARepositories struct {
	Marks SLAMarksRepository
}

// SLA is the periodic check of organization deadlines (run by cmd/tasker).
// Overdue marks keep their status; is_overdue is computed on read. The
// check publishes mark.sla_breached for every overdue mark on every run:
// the event id is derived from (mark_id, sla_due_at), so the notifier
// creates the notification once and ignores the repeats.
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
// deadline has passed and returns the number of such marks.
func (uc *SLA) ExpireOverdue(ctx context.Context) (int, error) {
	const op = "usecase.SLA.ExpireOverdue"

	marks, err := uc.repos.Marks.GetOverdueMarks(ctx, uc.now())
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	for _, mark := range marks {
		if !mark.OrganizationID.Valid || !mark.SLADueAt.Valid {
			continue
		}
		events.PublishEvent(ctx, uc.log, uc.events,
			events.NewMarkSLABreached(mark.ID, int(mark.OrganizationID.Int64), mark.SLADueAt.Time))
	}
	uc.log.Info("sla check finished", slog.String("op", op), slog.Int("overdue", len(marks)))

	return len(marks), nil
}
