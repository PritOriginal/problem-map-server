package apikeysrest

import "github.com/PritOriginal/problem-map-server/internal/models"

// CreateAPIKeyRequest is the JSON body of POST /api-keys.
type CreateAPIKeyRequest struct {
	// Name tells the keys apart in the list.
	Name string `json:"name" binding:"required,max=64" example:"city dashboard"`
	// ExpiresAt (RFC3339) makes the key stop working after that moment;
	// omitted means no expiry.
	ExpiresAt string `json:"expires_at" binding:"omitempty" example:"2027-01-01T00:00:00Z"`
}

// CreateAPIKeyResponse carries the record and the key itself: the key is
// shown only here, store it.
type CreateAPIKeyResponse struct {
	APIKey models.APIKey `json:"api_key"`
	Key    string        `json:"key" example:"pm_live_0123456789abcdef0123456789abcdef"`
}

// GetAPIKeysQuery is bound from the query string of GET /api-keys.
type GetAPIKeysQuery struct {
	// All lists every key (admin only).
	All bool `form:"all"`
}

type GetAPIKeysResponse struct {
	APIKeys []models.APIKey `json:"api_keys"`
}

type DeleteAPIKeyResponse struct {
	APIKeyId int `json:"api_key_id"`
}
