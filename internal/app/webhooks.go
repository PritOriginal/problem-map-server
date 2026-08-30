package app

import (
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/webhooks"
)

// NewWebhookSender builds the HTTPS sender and the URL policy of outgoing
// webhooks from cfg. With AllowPrivateURLs the SSRF guard is off, which is
// logged loudly because it must never be the case in production.
func NewWebhookSender(log *slog.Logger, cfg config.WebhooksConfig) (*webhooks.Sender, webhooks.URLPolicy) {
	policy := webhooks.URLPolicy{AllowPrivate: cfg.AllowPrivateURLs}
	if cfg.AllowPrivateURLs {
		log.Warn("webhooks.allow-private-urls is on: webhooks may target private and loopback addresses")
	}
	sender := webhooks.NewSender(webhooks.SenderOptions{
		Timeout: cfg.Timeout,
		Policy:  policy,
	})
	return sender, policy
}
