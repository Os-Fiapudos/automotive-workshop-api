package vehicle

import (
	"context"

	"github.com/google/uuid"
)

// CustomerLookup lets VehicleService check a referenced customer's
// existence/status without importing internal/features/customer directly
// (CLAUDE.md §9's "no feature imports another feature's package" rule) —
// mirrors how internal/features/auth.Service depends on the UserFinder
// interface it declares itself. cmd/api/main.go, which already imports both
// features, supplies the concrete implementation.
type CustomerLookup interface {
	// IsActiveCustomer reports whether customerID refers to an existing
	// customer and, if so, whether that customer is currently ACTIVE.
	IsActiveCustomer(ctx context.Context, customerID uuid.UUID) (found bool, active bool, err error)
}

// VehicleService orchestrates the vehicle use cases on top of a
// VehicleRepository and a CustomerLookup. There is one method per use case;
// no CQRS-style command/query split (mirrors
// specs/customer-management/design.md §1.1's CustomerService).
type VehicleService struct {
	repository VehicleRepository
	customers  CustomerLookup
}

func NewVehicleService(repository VehicleRepository, customers CustomerLookup) *VehicleService {
	return &VehicleService{repository: repository, customers: customers}
}

// Create validates and persists a new, always-ACTIVE vehicle, after
// confirming its customerId references an existing, ACTIVE customer
// (requirements.md BR1).
func (service *VehicleService) Create(ctx context.Context, rawLicensePlate, brand, model string, year int, color string, customerID uuid.UUID) (*Vehicle, error) {
	found, active, err := service.customers.IsActiveCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrCustomerNotFound
	}
	if !active {
		return nil, ErrCustomerInactive
	}

	vehicle, err := NewVehicle(rawLicensePlate, brand, model, year, color, customerID)
	if err != nil {
		return nil, err
	}

	// Application-level pre-check for a clean 409 in the common case; the
	// database's unique index (see repository.Create) is the final
	// guarantee against a race between two concurrent requests.
	exists, err := service.repository.ExistsByPlate(ctx, vehicle.LicensePlate, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrDuplicatePlate
	}

	if err := service.repository.Create(ctx, vehicle); err != nil {
		return nil, err
	}
	return vehicle, nil
}

// Get retrieves a vehicle by id.
func (service *VehicleService) Get(ctx context.Context, id uuid.UUID) (*Vehicle, error) {
	return service.repository.FindByID(ctx, id)
}

// GetByPlate normalizes rawLicensePlate and retrieves the vehicle that owns
// it.
func (service *VehicleService) GetByPlate(ctx context.Context, rawLicensePlate string) (*Vehicle, error) {
	normalized := NormalizePlate(rawLicensePlate)
	return service.repository.FindByPlate(ctx, normalized)
}

// List retrieves a page of vehicles (active and inactive alike).
func (service *VehicleService) List(ctx context.Context, page, pageSize int) ([]*Vehicle, int, error) {
	return service.repository.List(ctx, page, pageSize)
}

// ListByCustomer retrieves a page of the given customer's vehicles (active
// and inactive alike). It reports ErrCustomerNotFound if customerID doesn't
// reference an existing customer; an existing but INACTIVE customer's
// vehicles are still listed (requirements.md BR8 applied to this read path).
func (service *VehicleService) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*Vehicle, int, error) {
	found, _, err := service.customers.IsActiveCustomer(ctx, customerID)
	if err != nil {
		return nil, 0, err
	}
	if !found {
		return nil, 0, ErrCustomerNotFound
	}

	return service.repository.ListByCustomer(ctx, customerID, page, pageSize)
}

// UpdateInput carries the partial PATCH fields; a nil field means "leave
// unchanged." License plate and customer id are deliberately absent —
// neither is mutable after creation (requirements.md §3.2).
type UpdateInput struct {
	Brand *string
	Model *string
	Year  *int
	Color *string
}

// Update loads the vehicle, applies only the fields present in input, and
// persists the result. If Year is present, it is re-validated against the
// valid range before being applied.
func (service *VehicleService) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (*Vehicle, error) {
	vehicle, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	brand, model, year, color := vehicle.Brand, vehicle.Model, vehicle.Year, vehicle.Color
	if input.Brand != nil {
		brand = *input.Brand
	}
	if input.Model != nil {
		model = *input.Model
	}
	if input.Year != nil {
		year = *input.Year
	}
	if input.Color != nil {
		color = *input.Color
	}

	if err := vehicle.UpdateDetails(brand, model, year, color); err != nil {
		return nil, err
	}

	if err := service.repository.Update(ctx, vehicle); err != nil {
		return nil, err
	}
	return vehicle, nil
}

// Deactivate logically deactivates a vehicle (never a physical delete).
// Deactivating an already-inactive vehicle is a no-op, not an error.
func (service *VehicleService) Deactivate(ctx context.Context, id uuid.UUID) (*Vehicle, error) {
	vehicle, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	vehicle.Deactivate()

	if err := service.repository.Update(ctx, vehicle); err != nil {
		return nil, err
	}
	return vehicle, nil
}
