package serviceorder

import (
	"time"

	"github.com/google/uuid"
)

// StockMovementType is a StockMovement's direction: ENTRY (adds to stock) or
// EXIT (removes from stock).
type StockMovementType string

const (
	StockMovementEntry StockMovementType = "ENTRY"
	StockMovementExit  StockMovementType = "EXIT"
)

// StockMovement is a single change to a Product's stock balance, deducted
// for (EXIT) or restored to (ENTRY reversal) a ServiceOrder — the same
// stock_movements ledger internal/features/product's own manual adjustments
// write to, distinguished by ServiceOrderID (design.md §0). Read-only from
// this package's perspective except for the writes RegisterStockUsage/
// ReverseStockMovement perform.
type StockMovement struct {
	ID                 uuid.UUID
	ProductID          uuid.UUID
	ServiceOrderID     *uuid.UUID
	Type               StockMovementType
	Quantity           int
	PreviousStock      int
	NewStock           int
	Reason             string
	ReversedMovementID *uuid.UUID
	OccurredAt         time.Time
}

// StockUsageItem is one raw line of a usage-deduction request: a product and
// the quantity taken from it (requirements.md §3, BR3).
type StockUsageItem struct {
	ProductID string
	Quantity  int
}

// validateStockUsageItems enforces the business rules that don't need
// repository access: at least one item (BR7 implies a request has items to
// roll back), every quantity strictly positive (BR3). Existence, active
// status, and balance are enforced atomically by the repository (design.md
// §3) to avoid a check-then-act race with a concurrent request (BR8).
func validateStockUsageItems(items []StockUsageItem) error {
	if len(items) == 0 {
		return ErrEmptyStockUsage
	}
	for _, item := range items {
		if item.Quantity <= 0 {
			return ErrInvalidQuantity
		}
	}
	return nil
}
