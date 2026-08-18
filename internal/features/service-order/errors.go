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
)
