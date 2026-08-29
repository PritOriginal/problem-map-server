package reportsrest

import (
	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
)

// CreateReportRequest is the JSON body of POST /reports.
type CreateReportRequest struct {
	TargetType string `json:"target_type" binding:"required,oneof=mark check comment"`
	TargetID   int    `json:"target_id" binding:"required,min=1"`
	Reason     string `json:"reason" binding:"required,oneof=spam offensive wrong_place duplicate other"`
	// Comment is limited to models.MaxReportCommentLen runes (the binding
	// tag cannot reference the constant; a test keeps them in sync).
	Comment string `json:"comment" binding:"max=1000"`
}

// Model converts the request to a report filed by reporterId.
func (r CreateReportRequest) Model(reporterId int) models.Report {
	return models.Report{
		ReporterID: reporterId,
		TargetType: models.ReportTargetType(r.TargetType),
		TargetID:   r.TargetID,
		Reason:     models.ReportReason(r.Reason),
		Comment:    r.Comment,
	}
}

type CreateReportResponse struct {
	Report models.Report `json:"report"`
}

// GetQueueRequest is bound from the query string of GET /moderation/queue.
type GetQueueRequest struct {
	listquery.Pagination
	Status     string `form:"status" binding:"omitempty,oneof=open resolved dismissed"`
	TargetType string `form:"target_type" binding:"omitempty,oneof=mark check comment"`
}

// Filters converts the request to domain filters; an empty status means
// open reports (the queue).
func (r GetQueueRequest) Filters() models.GetReportsFilters {
	status := models.ReportStatus(r.Status)
	if status == "" {
		status = models.ReportStatusOpen
	}
	return models.GetReportsFilters{
		Status:     status,
		TargetType: models.ReportTargetType(r.TargetType),
		Pagination: r.Model(),
	}
}

type GetQueueResponse struct {
	Reports []models.ReportWithTarget `json:"reports"`
}

type GetMyReportsResponse struct {
	Reports []models.Report `json:"reports"`
}

// ResolveReportRequest is the JSON body of PATCH /moderation/reports/{id}.
type ResolveReportRequest struct {
	Status string `json:"status" binding:"required,oneof=resolved dismissed"`
}

type ResolveReportResponse struct {
	Report models.Report `json:"report"`
}
