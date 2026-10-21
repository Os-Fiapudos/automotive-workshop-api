package domain

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
