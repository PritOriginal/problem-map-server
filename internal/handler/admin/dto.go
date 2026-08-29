package adminrest

import (
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/guregu/null/v6"
)

// SettingsResponse is the payload of GET/PUT /admin/settings: the runtime
// settings document itself.
type SettingsResponse = usecase.RuntimeSettings

// UpdateSettingsRequest is the JSON body of PUT /admin/settings: the
// settings document. It is decoded over the current values, so an omitted
// field keeps its value; every present field is range-validated.
type UpdateSettingsRequest = usecase.RuntimeSettings

// HistoryRequest is bound from the query string of GET /admin/settings/history.
type HistoryRequest struct {
	Limit int `form:"limit,default=20" binding:"min=1,max=100"`
}

type SettingsHistoryResponse struct {
	Changes []models.SettingChange `json:"changes"`
}

type MarkTypesResponse struct {
	MarkTypes []models.MarkType `json:"mark_types"`
}

// MarkTypeResponse is the payload of POST/PATCH /admin/mark-types: the
// mark type itself, with name_ru/name_en.
type MarkTypeResponse = models.MarkType

// CreateMarkTypeRequest is the JSON body of POST /admin/mark-types.
type CreateMarkTypeRequest struct {
	Code   string `json:"code" binding:"required,max=40"`
	NameRU string `json:"name_ru" binding:"required,max=40"`
	NameEN string `json:"name_en" binding:"max=40"`
	// Icon is a client-side icon identifier.
	Icon *string `json:"icon" binding:"omitempty,max=64"`
	// Color is a hex colour like "#ff8800".
	Color    *string `json:"color" binding:"omitempty,len=7"`
	SLAHours int     `json:"sla_hours" binding:"required,min=1"`
}

func (r CreateMarkTypeRequest) Model() models.MarkTypeCreate {
	return models.MarkTypeCreate{
		Code:     r.Code,
		NameRU:   r.NameRU,
		NameEN:   r.NameEN,
		Icon:     null.StringFromPtr(r.Icon),
		Color:    null.StringFromPtr(r.Color),
		SLAHours: r.SLAHours,
	}
}

// UpdateMarkTypeRequest is the JSON body of PATCH /admin/mark-types/{id};
// omitted fields are left unchanged, an empty icon/color clears it.
type UpdateMarkTypeRequest struct {
	Code      *string `json:"code" binding:"omitempty,max=40"`
	NameRU    *string `json:"name_ru" binding:"omitempty,max=40"`
	NameEN    *string `json:"name_en" binding:"omitempty,max=40"`
	Icon      *string `json:"icon" binding:"omitempty,max=64"`
	Color     *string `json:"color" binding:"omitempty,max=7"`
	SLAHours  *int    `json:"sla_hours" binding:"omitempty,min=1"`
	Active    *bool   `json:"active"`
	SortOrder *int    `json:"sort_order" binding:"omitempty,min=0"`
}

func (r UpdateMarkTypeRequest) Model() models.MarkTypeUpdate {
	return models.MarkTypeUpdate{
		Code:      r.Code,
		NameRU:    r.NameRU,
		NameEN:    r.NameEN,
		Icon:      r.Icon,
		Color:     r.Color,
		SLAHours:  r.SLAHours,
		Active:    r.Active,
		SortOrder: r.SortOrder,
	}
}
