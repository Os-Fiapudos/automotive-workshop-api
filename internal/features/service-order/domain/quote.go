package domain

import (
	"time"

	"github.com/google/uuid"
)

// QuoteStatus is a Quote's decision status, in English like every other
// domain enum — see docs/entities.md.
type QuoteStatus string

const (
	QuoteStatusPending  QuoteStatus = "PENDING"
	QuoteStatusApproved QuoteStatus = "APPROVED"
	QuoteStatusRejected QuoteStatus = "REJECTED"
)

// QuoteItemKind distinguishes a product/part line from a service line within
// a Quote.
type QuoteItemKind string

const (
	QuoteItemProduct QuoteItemKind = "PRODUCT"
	QuoteItemService QuoteItemKind = "SERVICE"
)

// QuoteItem is a resolved, priced quote line — the snapshot that gets
// persisted. Description and UnitPrice are copied from the catalog at
// composition time and never change afterward, even if the catalog does
// (requirements.md §3.5).
type QuoteItem struct {
	Kind        QuoteItemKind
	ProductID   uuid.UUID
	ServiceID   uuid.UUID
	Description string
	Quantity    int
	UnitPrice   float64
	Total       float64
}

// Quote is the priced budget for a ServiceOrder, composed from diagnosed
// items (requirements.md, RF05/RF06). Version/SentAt/SentVersion are added by
// specs/service-order-quote-decision/: Version increments on every
// compose/recompose (specs/service-order-diagnosis-quote/); SentAt/
// SentVersion record when the quote was sent to the customer and which
// Version was actually presented, satisfying that feature's requirement to
// register "a data de envio e a versão efetivamente apresentada".
type Quote struct {
	ID             uuid.UUID
	Code           int64
	ServiceOrderID uuid.UUID
	TotalAmount    float64
	Status         QuoteStatus
	Version        int
	Items          []QuoteItem
	GeneratedAt    time.Time
	SentAt         *time.Time
	SentVersion    *int
	RespondedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ValidateQuoteItems enforces requirements.md §3.3/§3.6: at least one item,
// every quantity strictly positive.
func ValidateQuoteItems(items []QuoteItem) error {
	if len(items) == 0 {
		return ErrEmptyQuote
	}
	for _, item := range items {
		if item.Quantity <= 0 {
			return ErrInvalidQuantity
		}
	}
	return nil
}

// CalculateTotal sums every item's Total. This is the single place RF06
// ("the total must be calculated exclusively by the back end") is
// implemented — no other code path computes a quote total.
func CalculateTotal(items []QuoteItem) float64 {
	var total float64
	for _, item := range items {
		total += item.Total
	}
	return total
}
