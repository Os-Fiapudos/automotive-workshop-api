package serviceorder

import "context"

// QuoteNotification is the data a QuoteNotifier needs to tell a customer their
// quote is available. It is an explicit DTO rather than the ServiceOrder
// aggregate because ServiceOrder only carries CustomerID — the recipient is
// resolved by the service layer (specs/service-order-email-notification/).
// It deliberately has no customer name and no tracking token: the token is
// stored only as a hash and cannot be rebuilt here.
type QuoteNotification struct {
	OrderCode      int64
	TotalAmount    float64
	RecipientEmail string
}

// QuoteNotifier is the notification port
// specs/service-order-quote-decision/requirements.md asks for. SendQuote calls
// it on a best-effort basis after the send is already durably recorded — a
// notification failure must never undo or block a legitimate send.
// The service only calls it with a non-empty RecipientEmail.
type QuoteNotifier interface {
	NotifyQuoteSent(ctx context.Context, notification QuoteNotification) error
}

// NoOpQuoteNotifier is the QuoteNotifier wired when no e-mail provider is
// configured: it does nothing.
type NoOpQuoteNotifier struct{}

func (NoOpQuoteNotifier) NotifyQuoteSent(context.Context, QuoteNotification) error { return nil }
