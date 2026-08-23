package customer

import (
	"context"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/document"
)

// CustomerService orchestrates the customer use cases on top of a
// CustomerRepository. There is one method per use case; no CQRS-style
// command/query split (see specs/customer-management/design.md §1.1).
type CustomerService struct {
	repository CustomerRepository
}

func NewCustomerService(repository CustomerRepository) *CustomerService {
	return &CustomerService{repository: repository}
}

// Create validates and persists a new, always-ACTIVE customer.
func (service *CustomerService) Create(ctx context.Context, name, rawDocument, phone string, email *string) (*Customer, error) {
	customer, err := NewCustomer(name, rawDocument, phone, email)
	if err != nil {
		return nil, err
	}

	// Application-level pre-check for a clean 409 in the common case; the
	// database's unique index (see repository.Create) is the final
	// guarantee against a race between two concurrent requests.
	exists, err := service.repository.ExistsByDocument(ctx, customer.Document.Value, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrDuplicateDocument
	}

	if err := service.repository.Create(ctx, customer); err != nil {
		return nil, err
	}
	return customer, nil
}

// Get retrieves a customer by id.
func (service *CustomerService) Get(ctx context.Context, id uuid.UUID) (*Customer, error) {
	return service.repository.FindByID(ctx, id)
}

// GetByDocument normalizes rawDocument and retrieves the customer that owns
// it (requirements.md §5, "the document must be normalized before query").
func (service *CustomerService) GetByDocument(ctx context.Context, rawDocument string) (*Customer, error) {
	normalized := document.Normalize(rawDocument)
	return service.repository.FindByDocument(ctx, normalized)
}

// List retrieves a page of customers (active and inactive alike).
func (service *CustomerService) List(ctx context.Context, page, pageSize int) ([]*Customer, int, error) {
	return service.repository.List(ctx, page, pageSize)
}

// UpdateInput carries the partial PATCH fields; a nil field means "leave
// unchanged."
type UpdateInput struct {
	Name     *string
	Document *string
	Phone    *string
	Email    *string
}

// Update loads the customer, applies only the fields present in input, and
// persists the result. If Document is present, it goes through
// normalization, validation, and a fresh uniqueness check against every
// other customer before being applied.
func (service *CustomerService) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (*Customer, error) {
	customer, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		customer.Name = *input.Name
	}
	if input.Phone != nil {
		customer.Phone = *input.Phone
	}
	if input.Email != nil {
		customer.Email = input.Email
	}
	if input.Document != nil {
		if err := customer.ChangeDocument(*input.Document); err != nil {
			return nil, err
		}

		exists, err := service.repository.ExistsByDocument(ctx, customer.Document.Value, &customer.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrDuplicateDocument
		}
	}

	if err := service.repository.Update(ctx, customer); err != nil {
		return nil, err
	}
	return customer, nil
}

// Deactivate logically deactivates a customer (never a physical delete).
// Deactivating an already-inactive customer is a no-op, not an error.
func (service *CustomerService) Deactivate(ctx context.Context, id uuid.UUID) (*Customer, error) {
	customer, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	customer.Deactivate()

	if err := service.repository.Update(ctx, customer); err != nil {
		return nil, err
	}
	return customer, nil
}
