package models

import (
	"encoding/json"
	"time"

	"github.com/guregu/null/v6"
)

// Setting is a stored runtime setting: a JSON document under a key.
type Setting struct {
	Key       string          `json:"key" db:"key"`
	Value     json.RawMessage `json:"value" db:"value" swaggertype:"object"`
	UpdatedBy null.Int        `json:"updated_by" db:"updated_by" swaggertype:"integer"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// SettingChange is one row of settings_history: the value of Key before
// (nil on the first write) and after the change.
type SettingChange struct {
	ID  int64  `json:"id" db:"id"`
	Key string `json:"key" db:"key"`
	// Old is nil for the first write of the key.
	Old       *json.RawMessage `json:"old" db:"old" swaggertype:"object"`
	New       json.RawMessage  `json:"new" db:"new" swaggertype:"object"`
	UpdatedBy null.Int         `json:"updated_by" db:"updated_by" swaggertype:"integer"`
	UpdatedAt time.Time        `json:"updated_at" db:"updated_at"`
}

// MarkTypeCreate is the input of POST /admin/mark-types.
type MarkTypeCreate struct {
	Code     string
	NameRU   string
	NameEN   string
	Icon     null.String
	Color    null.String
	SLAHours int
}

// MarkTypeUpdate is the input of PATCH /admin/mark-types/{id}; nil fields
// are left unchanged. An empty Icon or Color clears the attribute.
type MarkTypeUpdate struct {
	Code      *string
	NameRU    *string
	NameEN    *string
	Icon      *string
	Color     *string
	SLAHours  *int
	Active    *bool
	SortOrder *int
}

// IsEmpty reports whether the update changes nothing.
func (u MarkTypeUpdate) IsEmpty() bool {
	return u.Code == nil && u.NameRU == nil && u.NameEN == nil && u.Icon == nil && u.Color == nil &&
		u.SLAHours == nil && u.Active == nil && u.SortOrder == nil
}
