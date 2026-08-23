package vehicle

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

// fakeRepository is an in-memory VehicleRepository used only by the
// service-level unit tests in this package — no mocking framework, per
// specs/vehicle-management/design.md §6.
type fakeRepository struct {
	byID map[uuid.UUID]*Vehicle
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byID: make(map[uuid.UUID]*Vehicle)}
}

func (fake *fakeRepository) Create(_ context.Context, vehicle *Vehicle) error {
	for _, existing := range fake.byID {
		if existing.LicensePlate == vehicle.LicensePlate {
			return ErrDuplicatePlate
		}
	}
	vehicle.Code = int64(len(fake.byID) + 1)
	fake.byID[vehicle.ID] = vehicle
	return nil
}

func (fake *fakeRepository) FindByID(_ context.Context, id uuid.UUID) (*Vehicle, error) {
	vehicle, ok := fake.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	vehicleCopy := *vehicle
	return &vehicleCopy, nil
}

func (fake *fakeRepository) FindByPlate(_ context.Context, normalizedPlate string) (*Vehicle, error) {
	for _, vehicle := range fake.byID {
		if vehicle.LicensePlate == normalizedPlate {
			vehicleCopy := *vehicle
			return &vehicleCopy, nil
		}
	}
	return nil, ErrNotFound
}

func (fake *fakeRepository) ExistsByPlate(_ context.Context, normalizedPlate string, excludeID *uuid.UUID) (bool, error) {
	for _, vehicle := range fake.byID {
		if vehicle.LicensePlate != normalizedPlate {
			continue
		}
		if excludeID != nil && vehicle.ID == *excludeID {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (fake *fakeRepository) List(_ context.Context, page, pageSize int) ([]*Vehicle, int, error) {
	all := make([]*Vehicle, 0, len(fake.byID))
	for _, vehicle := range fake.byID {
		all = append(all, vehicle)
	}
	return paginate(all, page, pageSize)
}

func (fake *fakeRepository) ListByCustomer(_ context.Context, customerID uuid.UUID, page, pageSize int) ([]*Vehicle, int, error) {
	var filtered []*Vehicle
	for _, vehicle := range fake.byID {
		if vehicle.CustomerID == customerID {
			filtered = append(filtered, vehicle)
		}
	}
	return paginate(filtered, page, pageSize)
}

func paginate(all []*Vehicle, page, pageSize int) ([]*Vehicle, int, error) {
	sort.Slice(all, func(first, second int) bool { return all[first].Code < all[second].Code })

	total := len(all)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

func (fake *fakeRepository) Update(_ context.Context, vehicle *Vehicle) error {
	if _, ok := fake.byID[vehicle.ID]; !ok {
		return ErrNotFound
	}
	fake.byID[vehicle.ID] = vehicle
	return nil
}

// fakeCustomerLookup is an in-memory CustomerLookup used only by the
// service-level unit tests in this package.
type fakeCustomerLookup struct {
	activeByID map[uuid.UUID]bool
}

func newFakeCustomerLookup() *fakeCustomerLookup {
	return &fakeCustomerLookup{activeByID: make(map[uuid.UUID]bool)}
}

func (fake *fakeCustomerLookup) addActive(id uuid.UUID) {
	fake.activeByID[id] = true
}

func (fake *fakeCustomerLookup) addInactive(id uuid.UUID) {
	fake.activeByID[id] = false
}

func (fake *fakeCustomerLookup) IsActiveCustomer(_ context.Context, id uuid.UUID) (bool, bool, error) {
	active, found := fake.activeByID[id]
	return found, active, nil
}
