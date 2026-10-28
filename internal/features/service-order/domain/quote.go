package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/money"
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
	UnitPrice   money.Cents
	Total       money.Cents
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
	TotalAmount    money.Cents
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

// PriceQuoteItems sets every item's Total to UnitPrice × Quantity, in exact
// integer cents (specs/monetary-value-precision/). A product beyond
// NUMERIC(12,2) is ErrQuoteAmountOutOfRange; it runs after
// ValidateQuoteItems, so quantities are already known to be positive.
func PriceQuoteItems(items []QuoteItem) error {
	for index := range items {
		total, err := items[index].UnitPrice.MulQuantity(items[index].Quantity)
		if err != nil {
			return quoteAmountError(err)
		}
		items[index].Total = total
	}
	return nil
}

// CalculateTotal sums every item's Total. This is the single place RF06
// ("the total must be calculated exclusively by the back end") is
// implemented — no other code path computes a quote total. The sum is exact
// integer-cents arithmetic, bounded by quotes.total_amount's NUMERIC(12,2)
// (ErrQuoteAmountOutOfRange beyond it).
func CalculateTotal(items []QuoteItem) (money.Cents, error) {
	var total money.Cents
	for _, item := range items {
		var err error
		if total, err = total.Add(item.Total); err != nil {
			return 0, quoteAmountError(err)
		}
	}
	return total, nil
}

// quoteAmountError maps a money arithmetic rejection onto this feature's
// domain errors.
func quoteAmountError(err error) error {
	if errors.Is(err, money.ErrInvalidQuantity) {
		return ErrInvalidQuantity
	}
	return ErrQuoteAmountOutOfRange
}
