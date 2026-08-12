package customer

import "errors"

var (
	// ErrNotFound is returned when a customer cannot be located by id or
	// document.
	ErrNotFound = errors.New("customer not found")

	// ErrDuplicateDocument is returned when a document already belongs to a
	// different customer. Returned both from the application-level
	// pre-check and from the database unique-constraint violation, so a
	// race between two concurrent requests is still caught.
	ErrDuplicateDocument = errors.New("document already belongs to another customer")

	// ErrDuplicateEmail is returned when an e-mail already belongs to a
	// different customer. Unlike ErrDuplicateDocument, there is no
	// application-level pre-check for this — ux_customers_email (a
	// pre-existing database invariant, see docs/schema.sql) is the only
	// guard, mapped from its unique-constraint violation by
	// PostgresCustomerRepository (see requirements.md §3.4.1).
	ErrDuplicateEmail = errors.New("email already belongs to another customer")

	// ErrInvalidDocument wraps a CPF/CNPJ normalization/validation failure
	// so handlers can distinguish it (400) from other domain errors.
	ErrInvalidDocument = errors.New("invalid document")
)
