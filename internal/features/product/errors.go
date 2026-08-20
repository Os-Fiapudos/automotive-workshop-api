package product

import "errors"

var (
	// ErrNotFound is returned when no product exists with the requested id or code.
	ErrNotFound = errors.New("product not found")

	// ErrDuplicateCode is returned when attempting to create or update a product code that already exists.
	ErrDuplicateCode = errors.New("product code already belongs to another product")

	// ErrInvalidType is returned when the product type is not PART or SUPPLY.
	ErrInvalidType = errors.New("invalid product type, must be PART or SUPPLY")

	// ErrInvalidUnitPrice is returned when the unit price is negative.
	ErrInvalidUnitPrice = errors.New("unit price cannot be negative")

	// ErrInvalidStock is returned when the current stock quantity is negative.
	ErrInvalidStock = errors.New("current stock cannot be negative")

	// ErrStockDirectUpdateNotAllowed is returned when attempting to modify currentStock directly via cadastral update (RNF07).
	ErrStockDirectUpdateNotAllowed = errors.New("stock balance cannot be modified via cadastral update")

	// ErrInactiveProduct is returned when an inactive product is selected for a new quote or operation.
	ErrInactiveProduct = errors.New("inactive product cannot be used in new quotes or stock adjustments")

	// ErrProductInUse is returned when attempting to physically delete a product that is referenced in service orders or quotes.
	ErrProductInUse = errors.New("product is referenced in service orders or quotes and cannot be physically deleted")

	// ErrInsufficientStock is returned when an exit adjustment exceeds available stock balance.
	ErrInsufficientStock = errors.New("insufficient stock balance for exit adjustment")

	// ErrInvalidQuantity is returned when stock adjustment quantity is less than or equal to zero.
	ErrInvalidQuantity = errors.New("stock adjustment quantity must be greater than zero")

	// ErrEmptyReason is returned when no reason is provided for a stock adjustment.
	ErrEmptyReason = errors.New("stock adjustment reason is required")

	// ErrInvalidMovementType is returned when movement type is neither ENTRY nor EXIT.
	ErrInvalidMovementType = errors.New("invalid movement type, must be ENTRY or EXIT")
)
