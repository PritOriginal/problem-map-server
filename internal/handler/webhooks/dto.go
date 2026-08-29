package webhooksrest

import "github.com/PritOriginal/problem-map-server/internal/models"

// CreateWebhookRequest is the JSON body of POST /webhooks.
type CreateWebhookRequest struct {
	// URL must be https and point at a public host.
	URL string `json:"url" binding:"required,max=2048" example:"https://example.org/hooks/problem-map"`
	// Events are the subjects to receive: exact ("mark.status_changed"),
	// prefix ("mark.*") or "*".
	Events []string `json:"events" binding:"required,min=1,max=32,dive,min=1,max=64" example:"mark.status_changed,check.*"`
	// Secret signs the deliveries; generated when omitted and returned once.
	Secret string `json:"secret" binding:"omitempty,min=16,max=256"`
}

func (r CreateWebhookRequest) Model() models.Webhook {
	return models.Webhook{URL: r.URL, Events: r.Events, Secret: r.Secret}
}

// CreateWebhookResponse carries the webhook and its secret: the secret is
// shown only here, store it.
type CreateWebhookResponse struct {
	Webhook models.Webhook `json:"webhook"`
	Secret  string         `json:"secret"`
}

type GetWebhooksResponse struct {
	Webhooks []models.Webhook `json:"webhooks"`
}

// UpdateWebhookRequest is the JSON body of PATCH /webhooks/{id}; omitted
// fields are left unchanged.
type UpdateWebhookRequest struct {
	Active *bool    `json:"active"`
	Events []string `json:"events" binding:"omitempty,min=1,max=32,dive,min=1,max=64"`
}

func (r UpdateWebhookRequest) Model() models.WebhookUpdate {
	return models.WebhookUpdate{Active: r.Active, Events: r.Events}
}

type WebhookResponse struct {
	Webhook models.Webhook `json:"webhook"`
}

type DeleteWebhookResponse struct {
	WebhookId int `json:"webhook_id"`
}

type GetDeliveriesResponse struct {
	Deliveries []models.WebhookDelivery `json:"deliveries"`
}

// TestWebhookResponse reports the outcome of the test delivery.
type TestWebhookResponse struct {
	Delivery models.WebhookDelivery `json:"delivery"`
}
