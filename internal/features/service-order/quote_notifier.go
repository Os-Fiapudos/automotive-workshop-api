package serviceorder

import (
	"context"

	"automotive-workshop-api/internal/features/service-order/usecase"
)

// NoOpQuoteNotifier is the usecase.QuoteNotifier wired when no e-mail
// provider is configured: it does nothing.
type NoOpQuoteNotifier struct{}

func (NoOpQuoteNotifier) NotifyQuoteSent(context.Context, usecase.QuoteNotification) error {
	return nil
}
