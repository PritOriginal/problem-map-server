package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/guregu/null/v6"
)

// ReportTargetType is what a report is about.
type ReportTargetType string

const (
	ReportTargetMark    ReportTargetType = "mark"
	ReportTargetCheck   ReportTargetType = "check"
	ReportTargetComment ReportTargetType = "comment"
)

// Valid reports whether t is a known target type.
func (t ReportTargetType) Valid() bool {
	switch t {
	case ReportTargetMark, ReportTargetCheck, ReportTargetComment:
		return true
	default:
		return false
	}
}

// ReportReason is why the reporter complains.
type ReportReason string

const (
	ReportReasonSpam       ReportReason = "spam"
	ReportReasonOffensive  ReportReason = "offensive"
	ReportReasonWrongPlace ReportReason = "wrong_place"
	ReportReasonDuplicate  ReportReason = "duplicate"
	ReportReasonOther      ReportReason = "other"
)

// Valid reports whether r is a known reason.
func (r ReportReason) Valid() bool {
	switch r {
	case ReportReasonSpam, ReportReasonOffensive, ReportReasonWrongPlace, ReportReasonDuplicate, ReportReasonOther:
		return true
	default:
		return false
	}
}

// ReportStatus is the moderation state of a report.
type ReportStatus string

const (
	ReportStatusOpen      ReportStatus = "open"
	ReportStatusResolved  ReportStatus = "resolved"
	ReportStatusDismissed ReportStatus = "dismissed"
)

// Valid reports whether s is a known status.
func (s ReportStatus) Valid() bool {
	switch s {
	case ReportStatusOpen, ReportStatusResolved, ReportStatusDismissed:
		return true
	default:
		return false
	}
}

// Final reports whether s is a moderator's decision (resolved or dismissed).
func (s ReportStatus) Final() bool {
	return s == ReportStatusResolved || s == ReportStatusDismissed
}

// MaxReportCommentLen is the maximum length (in runes) of a report comment.
const MaxReportCommentLen = 1000

// Report is a user's complaint about a mark, a check or a comment. One
// reporter may report a target once (UNIQUE (reporter_id, target_type,
// target_id)).
type Report struct {
	ID         int              `json:"report_id" db:"report_id"`
	ReporterID int              `json:"reporter_id" db:"reporter_id"`
	TargetType ReportTargetType `json:"target_type" db:"target_type"`
	TargetID   int              `json:"target_id" db:"target_id"`
	Reason     ReportReason     `json:"reason" db:"reason"`
	Comment    string           `json:"comment" db:"comment"`
	Status     ReportStatus     `json:"status" db:"status"`
	ResolvedBy null.Int         `json:"resolved_by" db:"resolved_by" swaggertype:"integer"`
	ResolvedAt null.Time        `json:"resolved_at" db:"resolved_at" swaggertype:"string" format:"date-time"`
	CreatedAt  time.Time        `json:"created_at" db:"created_at"`
}

// Validate checks the target, the reason and the comment length.
func (r Report) Validate() error {
	if !r.TargetType.Valid() {
		return fmt.Errorf("unknown target_type %q", r.TargetType)
	}
	if r.TargetID <= 0 {
		return errors.New("target_id must be positive")
	}
	if !r.Reason.Valid() {
		return fmt.Errorf("unknown reason %q", r.Reason)
	}
	if len([]rune(r.Comment)) > MaxReportCommentLen {
		return fmt.Errorf("comment is longer than %d characters", MaxReportCommentLen)
	}
	return nil
}

// MarkBrief is the short form of a mark shown next to a report in the
// moderation queue.
type MarkBrief struct {
	ID           int            `json:"mark_id" db:"mark_id"`
	Description  string         `json:"description" db:"description"`
	Geom         *Point         `json:"geom" db:"geom"`
	MarkTypeID   int            `json:"mark_type_id" db:"type_mark_id"`
	MarkStatusID MarkStatusType `json:"mark_status_id" db:"mark_status_id"`
	UserID       int            `json:"user_id" db:"user_id"`
	Hidden       bool           `json:"hidden" db:"hidden"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
}

// ReportTarget describes what a report points at. Mark is filled for mark
// reports whose mark still exists; other target types carry only the id.
type ReportTarget struct {
	Type ReportTargetType `json:"type"`
	ID   int              `json:"id"`
	Mark *MarkBrief       `json:"mark,omitempty"`
}

// ReportWithTarget is a moderation queue item: the report plus its target.
type ReportWithTarget struct {
	Report
	Target ReportTarget `json:"target"`
}

// GetReportsFilters selects a page of reports.
type GetReportsFilters struct {
	// Status keeps reports in the status; empty means any.
	Status ReportStatus
	// TargetType keeps reports about one kind of target; empty means any.
	TargetType ReportTargetType
	// ReporterID keeps the reports of one user; 0 means any.
	ReporterID int

	Pagination Pagination
}

// Validate checks pagination and the enum filters.
func (f GetReportsFilters) Validate() error {
	if err := f.Pagination.Validate(); err != nil {
		return err
	}
	if f.Status != "" && !f.Status.Valid() {
		return fmt.Errorf("unknown status %q", f.Status)
	}
	if f.TargetType != "" && !f.TargetType.Valid() {
		return fmt.Errorf("unknown target_type %q", f.TargetType)
	}
	return nil
}
