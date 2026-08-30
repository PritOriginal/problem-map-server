package organizationsrest

import (
	"fmt"
	"mime/multipart"

	"github.com/PritOriginal/problem-map-server/internal/handler/listquery"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
)

// CreateOrganizationRequest is the JSON body of POST /organizations.
type CreateOrganizationRequest struct {
	Name        string `json:"name" binding:"required,max=255"`
	Description string `json:"description" binding:"max=4000"`
}

func (r CreateOrganizationRequest) Model() models.Organization {
	return models.Organization{Name: r.Name, Description: r.Description}
}

// UpdateOrganizationRequest is the JSON body of PATCH /organizations/{id};
// omitted fields are left unchanged.
type UpdateOrganizationRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=255"`
	Description *string `json:"description" binding:"omitempty,max=4000"`
}

func (r UpdateOrganizationRequest) Model() models.OrganizationUpdate {
	return models.OrganizationUpdate{Name: r.Name, Description: r.Description}
}

type OrganizationResponse struct {
	Organization models.Organization `json:"organization"`
}

type OrganizationDetailsResponse struct {
	Organization models.OrganizationDetails `json:"organization"`
}

type ListOrganizationsResponse struct {
	Organizations []models.OrganizationRef `json:"organizations"`
}

// AddMemberRequest is the JSON body of POST /organizations/{id}/members.
type AddMemberRequest struct {
	UserID int `json:"user_id" binding:"required,min=1"`
}

// MemberResponse reports the membership after POST/DELETE members.
type MemberResponse struct {
	OrganizationID int  `json:"organization_id"`
	UserID         int  `json:"user_id"`
	Member         bool `json:"member"`
}

// ResponsibilityRequest is the JSON body of POST/DELETE
// /organizations/{id}/responsibilities.
type ResponsibilityRequest struct {
	MarkTypeID int `json:"mark_type_id" binding:"required,min=1"`
	BoundaryID int `json:"boundary_id" binding:"required,min=1"`
}

func (r ResponsibilityRequest) Model(orgId int) models.OrganizationResponsibility {
	return models.OrganizationResponsibility{OrganizationID: orgId, MarkTypeID: r.MarkTypeID, BoundaryID: r.BoundaryID}
}

type ResponsibilityResponse struct {
	Responsibility models.OrganizationResponsibility `json:"responsibility"`
}

type RemoveResponsibilityResponse struct {
	OrganizationID int `json:"organization_id"`
	MarkTypeID     int `json:"mark_type_id"`
	BoundaryID     int `json:"boundary_id"`
}

// GetOrganizationMarksRequest is bound from the query string of
// GET /organizations/{id}/marks.
type GetOrganizationMarksRequest struct {
	listquery.Pagination
	StatusIds string `form:"status_ids"`
	Overdue   bool   `form:"overdue"`
}

func (r GetOrganizationMarksRequest) Filters() (models.GetOrganizationMarksFilters, error) {
	statusIds, err := handlers.ParseIntArray(r.StatusIds)
	if err != nil {
		return models.GetOrganizationMarksFilters{}, fmt.Errorf("failed parse status ids")
	}
	if len(statusIds) == 0 {
		statusIds = nil
	}
	return models.GetOrganizationMarksFilters{
		MarkStatusIds: statusIds,
		Overdue:       r.Overdue,
		Pagination:    r.Model(),
	}, nil
}

type GetOrganizationMarksResponse struct {
	Marks []models.Mark `json:"marks"`
}

type MarkResponse struct {
	Mark models.Mark `json:"mark"`
}

// ResolveMarkRequest is the multipart form of POST /marks/{id}/resolve.
type ResolveMarkRequest struct {
	Photos  []*multipart.FileHeader `form:"photos" binding:"required"`
	Comment string                  `form:"comment" binding:"max=1000"`
}

// AssignMarkRequest is the JSON body of PATCH /marks/{id}/assign.
type AssignMarkRequest struct {
	OrganizationID int `json:"organization_id" binding:"required,min=1"`
}
