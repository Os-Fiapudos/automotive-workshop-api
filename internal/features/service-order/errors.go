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

	// ErrServiceOrderNotFound is returned when the identified service order
	// does not exist.
	ErrServiceOrderNotFound = errors.New("service order not found")

	// ErrInvalidStatusTransition is returned when an operation requires the
	// order to be in a specific status it currently isn't (e.g. starting
	// diagnosis on an order that isn't RECEBIDA).
	ErrInvalidStatusTransition = errors.New("invalid service order status transition")

	// ErrDiagnosisNotStarted is returned when composing a quote is attempted
	// before diagnosis has started (order still RECEBIDA).
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

	// ErrProductNotFound is returned when a quote item references a product
	// id that does not exist in the catalog.
	ErrProductNotFound = errors.New("product not found")

	// ErrProductInactive is returned when a quote item references a product
	// that exists but is not ACTIVE.
	ErrProductInactive = errors.New("product is inactive")
)
