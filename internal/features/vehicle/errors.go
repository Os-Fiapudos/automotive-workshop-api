package vehicle

import "errors"

var (
	// ErrNotFound is returned when a vehicle cannot be located by id or
	// license plate.
	ErrNotFound = errors.New("vehicle not found")

	// ErrDuplicatePlate is returned when a license plate already belongs to
	// a different vehicle. Returned both from the application-level
	// pre-check and from the database unique-constraint violation, so a
	// race between two concurrent requests is still caught.
	ErrDuplicatePlate = errors.New("license plate already belongs to another vehicle")

	// ErrInvalidPlate wraps a license plate normalization/validation
	// failure so handlers can distinguish it (400) from other domain
	// errors.
	ErrInvalidPlate = errors.New("invalid license plate")

	// ErrInvalidYear wraps a manufacturing year outside the valid range so
	// handlers can distinguish it (400) from other domain errors.
	ErrInvalidYear = errors.New("invalid manufacturing year")

	// ErrCustomerNotFound is returned when the customerId referenced by a
	// vehicle does not correspond to an existing customer.
	ErrCustomerNotFound = errors.New("customer not found")

	// ErrCustomerInactive is returned when the customerId referenced by a
	// vehicle corresponds to an existing but INACTIVE customer
	// (requirements.md BR1).
	ErrCustomerInactive = errors.New("customer is inactive")
)
