package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	pb "github.com/PritOriginal/problem-map-protos/gen/go"
	"github.com/guregu/null/v6"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AdminBoundary struct {
	Id         int           `json:"id" db:"id"`
	Name       string        `json:"name" db:"name"`
	AdminLevel int           `json:"admin_level" db:"admin_level"`
	Geom       *MultiPolygon `json:"geom" db:"geom"`
}

type GetAdminBoundaryFilters struct {
	AdminLevels []int
}

type AdminBoundaryMarksCount struct {
	Id               int    `json:"id" db:"boundary_id"`
	Name             string `json:"name" db:"boundary_name"`
	TotalCount       int    `json:"total_count" db:"total_count"`
	UnconfirmedCount int    `json:"unconfirmed_count" db:"unconfirmed_count"`
	ConfirmedCount   int    `json:"confirmed_count" db:"confirmed_count"`
	UnderReviewCount int    `json:"under_review_count" db:"under_review_count"`
	ClosedCount      int    `json:"closed_count" db:"closed_count"`
}

type GetAdminBoundaryMarksCountFilters struct {
	AdminLevels   []int
	MarkTypeIds   []int
	MarkStatusIds []int
	// DateRange bounds marks.created_at; zero bounds are ignored.
	DateRange
}

// Validate checks the date range.
func (f GetAdminBoundaryMarksCountFilters) Validate() error {
	return f.DateRange.Validate()
}

type Region struct {
	ID   int      `json:"region_id" db:"region_id"`
	Name string   `json:"name"`
	Geom *Polygon `json:"geom"`
}

func (r *Region) ToProtobufObject() *pb.Region {
	return &pb.Region{
		Id:   int64(r.ID),
		Name: r.Name,
		Geom: r.Geom.ToProtobufObject(),
	}
}

type City struct {
	ID       int      `json:"city_id" db:"city_id"`
	Name     string   `json:"name"`
	RegionID int      `json:"region_id" db:"region_id"`
	Geom     *Polygon `json:"geom"`
}

func (c *City) ToProtobufObject() *pb.City {
	return &pb.City{
		Id:       int64(c.ID),
		Name:     c.Name,
		RegionId: int64(c.RegionID),
		Geom:     c.Geom.ToProtobufObject(),
	}
}

type District struct {
	ID     int      `json:"district_id" db:"district_id"`
	Name   string   `json:"name"`
	CityID int      `json:"city_id"`
	Geom   *Polygon `json:"geom"`
}

func (d *District) ToProtobufObject() *pb.District {
	return &pb.District{
		Id:     int64(d.ID),
		Name:   d.Name,
		CityId: int64(d.CityID),
		Geom:   d.Geom.ToProtobufObject(),
	}
}

// MaxMarkDescriptionLen is the maximum length (in runes) of a mark
// description accepted by REST (binding max=256) and gRPC.
const MaxMarkDescriptionLen = 256

type Mark struct {
	ID           int            `json:"mark_id" db:"mark_id"`
	Description  string         `json:"description" db:"description"`
	Geom         *Point         `json:"geom" db:"geom"`
	MarkTypeID   int            `json:"mark_type_id" db:"type_mark_id"`
	MarkStatusID MarkStatusType `json:"mark_status_id" db:"mark_status_id"`
	UserID       int            `json:"user_id" db:"user_id"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
	// FollowersCount is the number of users following the mark.
	FollowersCount int `json:"followers_count" db:"followers_count"`
	// IsFollowing reports whether the viewer (see ContextWithViewer) follows
	// the mark; always false for anonymous requests.
	IsFollowing bool `json:"is_following" db:"is_following"`
	// OrganizationID is the city service assigned to resolve the mark; null
	// until the mark is confirmed and a responsible organization is found.
	OrganizationID null.Int `json:"organization_id" db:"organization_id" swaggertype:"integer"`
	// SLADueAt is the deadline the organization has to resolve the mark.
	SLADueAt null.Time `json:"sla_due_at" db:"sla_due_at" swaggertype:"string" format:"date-time"`
	// IsOverdue reports whether SLADueAt has passed while the mark is still
	// confirmed or in progress (computed on read).
	IsOverdue bool `json:"is_overdue" db:"is_overdue"`
	// CommentsCount is the number of comments on the mark that are not
	// deleted.
	CommentsCount int `json:"comments_count" db:"comments_count"`
}

// MarkUpdate lists the mark fields a client may change; nil means "keep".
type MarkUpdate struct {
	Description *string
	MarkTypeID  *int
}

// IsEmpty reports whether the update changes nothing.
func (u MarkUpdate) IsEmpty() bool {
	return u.Description == nil && u.MarkTypeID == nil
}

// Actor identifies who performs a mutation and what they are allowed to do.
type Actor struct {
	UserID int
	Role   Role
}

// IsModerator reports whether the actor may act on any mark.
func (a Actor) IsModerator() bool {
	return a.Role == RoleModerator || a.Role == RoleAdmin
}

// DefaultDedupRadiusM is the radius used by similar-mark search when the
// caller does not pass one.
const DefaultDedupRadiusM = 50

// GetSimilarMarksFilters selects active marks of the same type within
// RadiusM meters of a point (duplicate detection).
type GetSimilarMarksFilters struct {
	Lon        float64
	Lat        float64
	MarkTypeID int
	RadiusM    float64
	// ExcludeMarkID is skipped in the result (0 means none), e.g. the mark
	// being edited.
	ExcludeMarkID int
}

// Validate checks the point, the type and the radius.
func (f GetSimilarMarksFilters) Validate() error {
	if err := ValidateLonLat(f.Lon, f.Lat); err != nil {
		return err
	}
	if f.MarkTypeID <= 0 {
		return errors.New("mark_type_id must be positive")
	}
	if math.IsNaN(f.RadiusM) || f.RadiusM <= 0 || f.RadiusM > MaxNearbyRadiusM {
		return fmt.Errorf("radius must be between 1 and %d meters", MaxNearbyRadiusM)
	}
	return nil
}

// ActiveMarkStatuses are the statuses in which a mark still describes an
// open problem; closed and refuted marks are ignored by duplicate search.
func ActiveMarkStatuses() []MarkStatusType {
	return []MarkStatusType{UnconfirmedStatus, ConfirmedStatus, UnderReviewStatus, RediscoveredStatus, InProgressStatus}
}

func (m *Mark) ToProtobufObject() *pb.Mark {
	return &pb.Mark{
		Id:          int64(m.ID),
		Description: m.Description,
		Geom:        m.Geom.ToProtobufObject(),
		MarkTypeId:  int64(m.MarkTypeID),
		UserId:      int64(m.UserID),
		CreatedAt:   timestamppb.New(m.CreatedAt),
		UpdatedAt:   timestamppb.New(m.UpdatedAt),
	}
}

// MaxMarksIDs caps the number of ids one GetMarks batch request may name.
const MaxMarksIDs = 100

type GetMarksFilters struct {
	// IDs restricts the result to the listed marks (at most MaxMarksIDs);
	// empty means no restriction.
	IDs           []int
	MarkTypeIds   []int
	MarkStatusIds []int
	// UserID filters by author; 0 means any user.
	UserID int
	// BBox restricts marks to a bounding box (ST_Intersects); nil means no restriction.
	BBox *BBox
	// CreatedFrom / CreatedTo bound created_at (inclusive); zero means unbounded.
	CreatedFrom time.Time
	CreatedTo   time.Time
	// Sort / Order default to created_at desc when empty.
	Sort  MarksSort
	Order SortOrder

	Pagination Pagination
}

// Validate checks pagination, sort keys, bbox and the date range.
func (f GetMarksFilters) Validate() error {
	if len(f.IDs) > MaxMarksIDs {
		return fmt.Errorf("ids: at most %d values", MaxMarksIDs)
	}
	if err := f.Pagination.Validate(); err != nil {
		return err
	}
	if err := f.Sort.Validate(); err != nil {
		return err
	}
	if err := f.Order.Validate(); err != nil {
		return err
	}
	if f.BBox != nil {
		if err := f.BBox.Validate(); err != nil {
			return err
		}
	}
	if !f.CreatedFrom.IsZero() && !f.CreatedTo.IsZero() && f.CreatedTo.Before(f.CreatedFrom) {
		return errors.New("created_to is before created_from")
	}
	return nil
}

// MaxNearbyRadiusM caps the radius accepted by nearby searches (50 km).
const MaxNearbyRadiusM = 50_000

// GetMarksNearbyFilters selects marks within RadiusM meters of a point.
type GetMarksNearbyFilters struct {
	Lon           float64
	Lat           float64
	RadiusM       float64
	MarkTypeIds   []int
	MarkStatusIds []int

	Pagination Pagination
}

// Validate checks pagination, the point and the radius.
func (f GetMarksNearbyFilters) Validate() error {
	if err := f.Pagination.Validate(); err != nil {
		return err
	}
	if err := ValidateLonLat(f.Lon, f.Lat); err != nil {
		return err
	}
	// NaN compares false against every bound, so it has to be rejected explicitly.
	if math.IsNaN(f.RadiusM) || f.RadiusM <= 0 || f.RadiusM > MaxNearbyRadiusM {
		return fmt.Errorf("radius must be between 1 and %d meters", MaxNearbyRadiusM)
	}
	return nil
}

// MarkWithDistance is a mark together with its distance (meters) from a query point.
type MarkWithDistance struct {
	Mark
	DistanceM float64 `json:"distance_m" db:"distance_m"`
}

type DistanceFromMarkToPoint struct {
	MarkId   int     `db:"mark_id"`
	UserId   int     `db:"user_id"`
	Distance float64 `db:"distance_km"`
}

type GetDistanceFromMarkToPointFilters struct {
	MarkStatusIds []MarkStatusType
	MaxRadius     int
}

// MarkType is a dictionary entry: Code is the stable machine-readable
// identifier, Name is localised (see Lang).
type MarkType struct {
	ID int `json:"id" db:"type_mark_id"`
	// LegacyID mirrors ID under the key older clients read; it is filled
	// by MarshalJSON. Deprecated: use ID (`id`).
	LegacyID int    `json:"mark_type_id" db:"-"`
	Code     string `json:"code" db:"code"`
	Name     string `json:"name" db:"name"`
	// SLAHours is the time an organization has to resolve a mark of the type.
	SLAHours int `json:"sla_hours" db:"sla_hours"`
}

// MarshalJSON emits the deprecated `mark_type_id` alias next to `id`.
func (t MarkType) MarshalJSON() ([]byte, error) {
	type plain MarkType
	t.LegacyID = t.ID
	return json.Marshal(plain(t))
}

func (t *MarkType) ToProtobufObject() *pb.MarkType {
	return &pb.MarkType{
		Id:   int64(t.ID),
		Name: t.Name,
	}
}

type MarkStatusType int

const (
	UnconfirmedStatus MarkStatusType = iota + 1
	ConfirmedStatus
	UnderReviewStatus
	RediscoveredStatus
	ClosedStatus
	RefutedStatus
	// InProgressStatus — «В работе»: the assigned organization started
	// resolving the mark (Confirmed -> InProgress -> UnderReview).
	InProgressStatus
)

// MarkStatus is a dictionary entry: Code is the stable machine-readable
// identifier, Name is localised (see Lang).
type MarkStatus struct {
	ID int `json:"id" db:"mark_status_id"`
	// LegacyID mirrors ID under the key older clients read; it is filled
	// by MarshalJSON. Deprecated: use ID (`id`).
	LegacyID int      `json:"mark_status_id" db:"-"`
	ParentId null.Int `json:"parent_id" db:"parent_id"`
	Code     string   `json:"code" db:"code"`
	Name     string   `json:"name" db:"name"`
}

// MarshalJSON emits the deprecated `mark_status_id` alias next to `id`.
func (s MarkStatus) MarshalJSON() ([]byte, error) {
	type plain MarkStatus
	s.LegacyID = s.ID
	return json.Marshal(plain(s))
}

func (s *MarkStatus) ToProtobufObject() *pb.MarkStatus {
	return &pb.MarkStatus{
		Id:   int64(s.ID),
		Name: s.Name,
	}
}

type MarkStatusHistoryItem struct {
	ID              int                        `json:"id" db:"id"`
	MarkID          int                        `json:"mark_id" db:"mark_id"`
	OldMarkStatusID null.Value[MarkStatusType] `json:"old_mark_status_id" db:"old_mark_status_id"`
	NewMarkStatusID MarkStatusType             `json:"new_mark_status_id" db:"new_mark_status_id"`
	ChangedAt       time.Time                  `json:"changed_at" db:"changed_at"`
	PrevId          null.Int                   `json:"prev_id" db:"prev_id"`

	Checks []Check `json:"checks"`
}

type Check struct {
	ID                      int            `json:"check_id" db:"check_id"`
	UserID                  int            `json:"user_id" db:"user_id"`
	Username                string         `json:"username" db:"username"`
	MarkID                  int            `json:"mark_id" db:"mark_id"`
	MarkStatusId            MarkStatusType `json:"mark_status_id" db:"mark_status_id"`
	MarkStatusHistoryItemId int            `json:"mark_status_history_id" db:"mark_status_history_id"`
	Result                  bool           `json:"result" db:"result"`
	Comment                 string         `json:"comment" db:"comment"`
	Photos                  []string       `json:"photos"`
	CreatedAt               time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at" db:"updated_at"`
}

func (c *Check) ToProtobufObject() *pb.Check {
	return &pb.Check{
		Id:        int64(c.ID),
		UserId:    int64(c.UserID),
		Username:  c.Username,
		MarkId:    int64(c.MarkID),
		Result:    c.Result,
		Comment:   c.Comment,
		Photos:    c.Photos,
		CreatedAt: timestamppb.New(c.CreatedAt),
		UpdatedAt: timestamppb.New(c.UpdatedAt),
	}
}
