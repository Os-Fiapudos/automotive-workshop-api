package customer

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

// fakeRepository is an in-memory CustomerRepository used only by the
// service-level unit tests in this package — no mocking framework, per
// specs/customer-management/design.md §6.
type fakeRepository struct {
	byID map[uuid.UUID]*Customer
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byID: make(map[uuid.UUID]*Customer)}
}

func (fake *fakeRepository) Create(_ context.Context, customer *Customer) error {
	for _, existing := range fake.byID {
		if existing.Document.Value == customer.Document.Value {
			return ErrDuplicateDocument
		}
	}
	customer.Code = int64(len(fake.byID) + 1)
	fake.byID[customer.ID] = customer
	return nil
}

func (fake *fakeRepository) FindByID(_ context.Context, id uuid.UUID) (*Customer, error) {
	customer, ok := fake.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	customerCopy := *customer
	return &customerCopy, nil
}

func (fake *fakeRepository) FindByDocument(_ context.Context, normalizedDocument string) (*Customer, error) {
	for _, customer := range fake.byID {
		if customer.Document.Value == normalizedDocument {
			customerCopy := *customer
			return &customerCopy, nil
		}
	}
	return nil, ErrNotFound
}

func (fake *fakeRepository) ExistsByDocument(_ context.Context, normalizedDocument string, excludeID *uuid.UUID) (bool, error) {
	for _, customer := range fake.byID {
		if customer.Document.Value != normalizedDocument {
			continue
		}
		if excludeID != nil && customer.ID == *excludeID {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (fake *fakeRepository) List(_ context.Context, page, pageSize int) ([]*Customer, int, error) {
	all := make([]*Customer, 0, len(fake.byID))
	for _, customer := range fake.byID {
		all = append(all, customer)
	}
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

func (fake *fakeRepository) Update(_ context.Context, customer *Customer) error {
	if _, ok := fake.byID[customer.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range fake.byID {
		if id != customer.ID && existing.Document.Value == customer.Document.Value {
			return ErrDuplicateDocument
		}
	}
	fake.byID[customer.ID] = customer
	return nil
}
