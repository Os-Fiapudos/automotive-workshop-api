package notifier

import (
	"context"
	"testing"

	"automotive-workshop-api/internal/features/service-order/usecase"
)

// The no-op notifier is what cmd/api wires when RESEND_API_KEY is unset: it
// must accept any notification without error.
func TestNoOpQuoteNotifierAcceptsEverything(t *testing.T) {
	var notifier usecase.QuoteNotifier = NoOpQuoteNotifier{}
	if err := notifier.NotifyQuoteSent(context.Background(), usecase.QuoteNotification{OrderCode: 1, RecipientEmail: "cliente@example.com"}); err != nil {
		t.Fatalf("NoOpQuoteNotifier returned %v", err)
	}
}
