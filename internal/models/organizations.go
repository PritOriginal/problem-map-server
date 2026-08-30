package models

import (
	"errors"
	"time"
)

// Organization is a city service that resolves confirmed marks of the types
// and inside the boundaries listed in its responsibilities.
type Organization struct {
	ID          int       `json:"id" db:"organization_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// OrganizationRef is the public view of an organization (GET /organizations).
type OrganizationRef struct {
	ID   int    `json:"id" db:"organization_id"`
	Name string `json:"name" db:"name"`
}

// OrganizationDetails is an organization together with its members and
// responsibilities (GET /organizations/{id}, GET /organizations/me).
type OrganizationDetails struct {
	Organization
	Members          []User                       `json:"members"`
	Responsibilities []OrganizationResponsibility `json:"responsibilities"`
}

// OrganizationUpdate lists the organization fields an admin may change;
// nil means "keep".
type OrganizationUpdate struct {
	Name        *string
	Description *string
}

// IsEmpty reports whether the update changes nothing.
func (u OrganizationUpdate) IsEmpty() bool {
	return u.Name == nil && u.Description == nil
}

// OrganizationResponsibility makes an organization responsible for marks of
// MarkTypeID located inside the admin boundary BoundaryID.
type OrganizationResponsibility struct {
	ID             int `json:"id" db:"id"`
	OrganizationID int `json:"organization_id" db:"organization_id"`
	MarkTypeID     int `json:"mark_type_id" db:"mark_type_id"`
	BoundaryID     int `json:"boundary_id" db:"boundary_id"`
}

// Validate checks the referenced ids.
func (r OrganizationResponsibility) Validate() error {
	if r.MarkTypeID <= 0 {
		return errors.New("mark_type_id must be positive")
	}
	if r.BoundaryID <= 0 {
		return errors.New("boundary_id must be positive")
	}
	return nil
}

// GetOrganizationMarksFilters selects the queue of an organization: its
// marks ordered overdue first, then by the nearest SLA deadline.
type GetOrganizationMarksFilters struct {
	MarkStatusIds []int
	// Overdue keeps only marks whose SLA deadline has passed.
	Overdue bool

	Pagination Pagination
}

// Validate checks pagination.
func (f GetOrganizationMarksFilters) Validate() error {
	return f.Pagination.Validate()
}

// SLAStatuses are the statuses in which the SLA deadline of a mark is
// running: the mark is assigned but not yet resolved by the organization.
func SLAStatuses() []MarkStatusType {
	return []MarkStatusType{ConfirmedStatus, InProgressStatus}
}
