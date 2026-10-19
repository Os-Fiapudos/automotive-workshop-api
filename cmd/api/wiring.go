package main

import (
	"net/http"

	serviceorder "automotive-workshop-api/internal/features/service-order"
	"automotive-workshop-api/internal/shared/config"
)

// buildQuoteNotifier picks the quote-sent notifier from configuration: the
// Resend adapter when RESEND_API_KEY is set (config.Load already guarantees a
// sender is set with it), otherwise the no-op notifier. Extracted from main so
// the composition is testable.
func buildQuoteNotifier(configuration config.Config, httpClient *http.Client) serviceorder.QuoteNotifier {
	if configuration.ResendAPIKey == "" {
		return serviceorder.NoOpQuoteNotifier{}
	}
	return serviceorder.NewResendQuoteNotifier(configuration.ResendAPIKey, configuration.ResendFromEmail, httpClient)
}
