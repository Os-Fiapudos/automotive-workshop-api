package serviceorder

import "context"

// QuoteNotifier is the notification port
// specs/service-order-quote-decision/requirements.md asks for ("Porta de
// notificação, caso o envio por e-mail seja implementado"). The MVP has no
// real e-mail integration ("O MVP deve registrar o envio mesmo que não
// exista integração real de e-mail"), so SendQuote calls this port on a
// best-effort basis after the send is already durably recorded — a
// notification failure must never undo or block a legitimate send.
type QuoteNotifier interface {
	NotifyQuoteSent(ctx context.Context, order *ServiceOrder, quote *Quote) error
}

// NoOpQuoteNotifier is the QuoteNotifier wired by default: it does nothing,
// satisfying the MVP's "no real e-mail integration required" requirement.
type NoOpQuoteNotifier struct{}

func (NoOpQuoteNotifier) NotifyQuoteSent(context.Context, *ServiceOrder, *Quote) error { return nil }
