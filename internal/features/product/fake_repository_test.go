package product

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

type fakeRepository struct {
	byID   map[uuid.UUID]*Product
	usedIn map[uuid.UUID]bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		byID:   make(map[uuid.UUID]*Product),
		usedIn: make(map[uuid.UUID]bool),
	}
}

func (fake *fakeRepository) Create(_ context.Context, product *Product) error {
	for _, existing := range fake.byID {
		if product.Code > 0 && existing.Code == product.Code {
			return ErrDuplicateCode
		}
	}
	if product.Code == 0 {
		product.Code = int64(len(fake.byID) + 1)
	}
	fake.byID[product.ID] = product
	return nil
}

func (fake *fakeRepository) FindByID(_ context.Context, id uuid.UUID) (*Product, error) {
	product, ok := fake.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	productCopy := *product
	return &productCopy, nil
}

func (fake *fakeRepository) FindByCode(_ context.Context, code int64) (*Product, error) {
	for _, product := range fake.byID {
		if product.Code == code {
			productCopy := *product
			return &productCopy, nil
		}
	}
	return nil, ErrNotFound
}

func (fake *fakeRepository) ExistsByCode(_ context.Context, code int64, excludeID *uuid.UUID) (bool, error) {
	for _, product := range fake.byID {
		if product.Code != code {
			continue
		}
		if excludeID != nil && product.ID == *excludeID {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (fake *fakeRepository) List(_ context.Context, page, pageSize int, productType *Type, status *Status) ([]*Product, int, error) {
	all := make([]*Product, 0, len(fake.byID))
	for _, product := range fake.byID {
		if productType != nil && product.Type != *productType {
			continue
		}
		if status != nil && product.Status != *status {
			continue
		}
		all = append(all, product)
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

func (fake *fakeRepository) Update(_ context.Context, product *Product) error {
	if _, ok := fake.byID[product.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range fake.byID {
		if id != product.ID && existing.Code == product.Code {
			return ErrDuplicateCode
		}
	}
	fake.byID[product.ID] = product
	return nil
}

func (fake *fakeRepository) AdjustStock(_ context.Context, id uuid.UUID, delta int) (*Product, error) {
	product, ok := fake.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	if !product.IsActive() {
		return nil, ErrInactiveProduct
	}
	if delta < 0 && product.CurrentStock+delta < 0 {
		return nil, ErrInsufficientStock
	}
	product.CurrentStock += delta
	productCopy := *product
	return &productCopy, nil
}

func (fake *fakeRepository) IsUsedInQuotesOrOrders(_ context.Context, id uuid.UUID) (bool, error) {
	return fake.usedIn[id], nil
}
