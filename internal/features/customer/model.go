package customer

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/document"
)

// Status is the customer's situation: ACTIVE or INACTIVE.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// Customer is the domain aggregate for this feature. A Customer cannot exist
// with an invalid or unnormalized document — Document is only ever set
// through NewCustomer/ChangeDocument, which validate via
// internal/shared/document.
type Customer struct {
	ID        uuid.UUID
	Code      int64
	Name      string
	Document  document.Document
	Phone     string
	Email     *string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewCustomer builds a new customer that always starts ACTIVE (see
// specs/customer-management/requirements.md §3.5). rawDocument is
// normalized and check-digit validated internally; Code/CreatedAt/UpdatedAt
// are assigned by the database on insert.
func NewCustomer(name, rawDocument, phone string, email *string) (*Customer, error) {
	validatedDocument, err := document.New(rawDocument)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}

	return &Customer{
		ID:       uuid.New(),
		Name:     name,
		Document: validatedDocument,
		Phone:    phone,
		Email:    email,
		Status:   StatusActive,
	}, nil
}

// ChangeDocument normalizes and validates rawDocument and, on success,
// replaces the customer's current document. It does not check uniqueness —
// that is the repository/service's responsibility.
func (customer *Customer) ChangeDocument(rawDocument string) error {
	validatedDocument, err := document.New(rawDocument)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	customer.Document = validatedDocument
	return nil
}

// Deactivate moves the customer to INACTIVE. It is idempotent: deactivating
// an already-inactive customer is a no-op, not an error. There is
// deliberately no Activate method — this feature does not support
// reactivating a customer (see
// specs/customer-management/requirements.md §3.7).
func (customer *Customer) Deactivate() {
	customer.Status = StatusInactive
}

// IsActive reports whether the customer is currently ACTIVE.
func (customer *Customer) IsActive() bool {
	return customer.Status == StatusActive
}
