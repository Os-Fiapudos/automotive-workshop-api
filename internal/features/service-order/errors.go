package serviceorder

import "errors"

var (
	// ErrInvalidAggregate is returned when NewServiceOrder is called with a
	// nil customer or vehicle id — a programming error in the caller (the
	// service layer must resolve real ids before constructing the
	// aggregate), not a user-facing validation failure.
	ErrInvalidAggregate = errors.New("service order requires a customer and a vehicle")

	// ErrCustomerNotFound is returned when the identified customer does not
	// exist.
	ErrCustomerNotFound = errors.New("customer not found")

	// ErrCustomerInactive is returned when the identified customer exists
	// but is not ACTIVE.
	ErrCustomerInactive = errors.New("customer is inactive")

	// ErrVehicleNotFound is returned when the identified vehicle does not
	// exist.
	ErrVehicleNotFound = errors.New("vehicle not found")

	// ErrVehicleInactive is returned when the identified vehicle exists but
	// is not ACTIVE.
	ErrVehicleInactive = errors.New("vehicle is inactive")

	// ErrVehicleNotOwnedByCustomer is returned when the identified vehicle
	// belongs to a different customer than the one identified in the
	// request.
	ErrVehicleNotOwnedByCustomer = errors.New("vehicle does not belong to the identified customer")

	// ErrRequestedServiceNotFound is returned when a requested service id
	// does not exist in the catalog.
	ErrRequestedServiceNotFound = errors.New("requested service not found")

	// ErrRequestedProductNotFound is returned when a requested product id
	// does not exist in the catalog or exists but is INACTIVE
	// (specs/service-order-opening/design.md §7.4 — both map to the same 404).
	ErrRequestedProductNotFound = errors.New("requested product not found or inactive")

	// ErrInvalidRequestedProductQuantity is returned when a requested
	// product's quantity is not a positive integer.
	ErrInvalidRequestedProductQuantity = errors.New("requested product quantity must be greater than zero")

	// ErrDuplicateRequestedProduct is returned when the same product id
	// appears more than once in requestedProducts — rejected explicitly,
	// before any write, rather than left to a mid-transaction primary-key
	// failure (specs/service-order-opening/design.md §7.4b).
	ErrDuplicateRequestedProduct = errors.New("requested product appears more than once")

	// ErrServiceOrderNotFound is returned when the identified service order
	// does not exist.
	ErrServiceOrderNotFound = errors.New("service order not found")

	// ErrInvalidStatusTransition is returned when an operation requires the
	// order to be in a specific status it currently isn't (e.g. starting
	// diagnosis on an order that isn't RECEIVED).
	ErrInvalidStatusTransition = errors.New("invalid service order status transition")

	// ErrDiagnosisNotStarted is returned when composing a quote is attempted
	// before diagnosis has started (order still RECEIVED).
	ErrDiagnosisNotStarted = errors.New("diagnosis has not started for this service order")

	// ErrQuoteAlreadyDecided is returned when attempting to compose a quote
	// that has already been APPROVED or REJECTED.
	ErrQuoteAlreadyDecided = errors.New("quote has already been decided and cannot be altered")

	// ErrQuoteNotFound is returned when reading a quote that has not been
	// composed yet.
	ErrQuoteNotFound = errors.New("quote not found")

	// ErrEmptyQuote is returned when composing a quote with no items.
	ErrEmptyQuote = errors.New("quote must have at least one item")

	// ErrInvalidQuantity is returned when a quote item's quantity is not
	// greater than zero.
	ErrInvalidQuantity = errors.New("item quantity must be greater than zero")

	// ErrQuoteAmountOutOfRange is returned when an item total (unit price ×
	// quantity) or the quote total would exceed 9999999999.99, the limit of
	// the NUMERIC(12,2) columns they are stored in
	// (specs/monetary-value-precision/).
	ErrQuoteAmountOutOfRange = errors.New("quote amount exceeds the maximum of 9999999999.99")

	// ErrProductNotFound is returned when a quote item references a product
	// id that does not exist in the catalog.
	ErrProductNotFound = errors.New("product not found")

	// ErrProductInactive is returned when a quote item references a product
	// that exists but is not ACTIVE.
	ErrProductInactive = errors.New("product is inactive")

	// ErrServiceExecutionNotFound is returned when finishing a service
	// execution that does not exist for the identified service order
	// (specs/service-order-execution/).
	ErrServiceExecutionNotFound = errors.New("service execution not found")

	// ErrServiceExecutionAlreadyFinished is returned when attempting to
	// finish a service execution that already has an end date.
	ErrServiceExecutionAlreadyFinished = errors.New("service execution has already been finished")

	// ErrServiceExecutionEndBeforeStart is returned when a service
	// execution's end date is before its start date (BR4).
	ErrServiceExecutionEndBeforeStart = errors.New("service execution end date cannot be before its start date")

	// ErrExecutionsNotCompleted is returned when finalizing a service order
	// whose required executions (one per approved-quote service line item)
	// are not all complete (BR5).
	ErrExecutionsNotCompleted = errors.New("service order has pending service executions and cannot be finalized")

	// ErrTrackingTokenInvalid is returned by the customer-facing quote
	// approve/reject endpoints (specs/service-order-quote-decision/) when the
	// supplied tracking token is missing, malformed, doesn't match, belongs
	// to a different order, or has been revoked — mirroring
	// service-order-tracking's ErrTokenInvalid (all of these are
	// deliberately indistinguishable from the caller's perspective, 401).
	ErrTrackingTokenInvalid = errors.New("tracking token is invalid, revoked, or does not match this service order")

	// ErrInvalidQuoteDecision is returned by DecideQuoteExternal when the
	// relayed decision is neither approved nor rejected
	// (specs/service-order-quote-decision/requirements.md AC18, 400).
	ErrInvalidQuoteDecision = errors.New("decision must be approved or rejected")

	// ErrUnsupportedExternalStatus is returned by UpdateStatusExternal when
	// the requested status is not one the external status-update channel
	// accepts (specs/service-order-status-update/requirements.md BR2, 400).
	ErrUnsupportedExternalStatus = errors.New("status must be IN_DIAGNOSIS, COMPLETED or DELIVERED")

	// ErrInvalidStatusUpdateNotes is returned by UpdateStatusExternal when
	// notes exceed maxStatusUpdateNotesLength after trimming (BR6, 400).
	ErrInvalidStatusUpdateNotes = errors.New("notes must be at most 500 characters")

	// ErrEmptyStockUsage is returned when a stock usage request has no items
	// (specs/service-order-stock-usage/).
	ErrEmptyStockUsage = errors.New("stock usage must include at least one item")

	// ErrInsufficientStock is returned when a usage deduction would take a
	// product's balance negative.
	ErrInsufficientStock = errors.New("insufficient stock balance for this product")

	// ErrStockMovementNotFound is returned when the identified stock
	// movement does not exist for the identified service order.
	ErrStockMovementNotFound = errors.New("stock movement not found")

	// ErrStockMovementAlreadyReversed is returned when attempting to reverse
	// a movement that has already been reversed.
	ErrStockMovementAlreadyReversed = errors.New("stock movement has already been reversed")

	// ErrStockMovementNotReversible is returned when attempting to reverse a
	// movement that is not an EXIT usage deduction (e.g. an ENTRY reversal
	// itself).
	ErrStockMovementNotReversible = errors.New("stock movement is not reversible")
)
