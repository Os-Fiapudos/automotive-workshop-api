package servicecatalog

import (
	"context"
	"strings"
)

// ValidationError carries a business-rule violation whose message is safe to
// return to the caller (mapped to HTTP 400 by the handler). Field names the
// offending request field, so the handler can report it as an apierror.Detail;
// it is empty when the violation is about the request as a whole.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// NewService is the input of a catalog registration. Code is optional (D1):
// when nil, the database generates the sequential code.
type NewService struct {
	Code          *int64
	Name          string
	Description   string
	Price         float64
	EstimatedTime *int
}

// Changes is a partial update; a nil field means "leave as is".
type Changes struct {
	Name          *string
	Description   *string
	Price         *float64
	EstimatedTime *int
}

// ListFilter narrows the listing; a nil Active returns active and inactive alike.
type ListFilter struct {
	Active *bool
}

type Store interface {
	Create(ctx context.Context, in NewService) (*Service, error)
	List(ctx context.Context, filter ListFilter) ([]Service, error)
	FindByID(ctx context.Context, id string) (*Service, error)
	Update(ctx context.Context, id string, changes Changes) (*Service, error)
	Deactivate(ctx context.Context, id string) error
}

// Catalog holds the catalog's business rules. It is named Catalog, not Service,
// because Service is the domain entity of this slice (docs/entities.md).
type Catalog struct {
	store Store
}

func NewCatalog(store Store) *Catalog { return &Catalog{store: store} }

func (c *Catalog) Create(ctx context.Context, in NewService) (*Service, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)

	if in.Name == "" {
		return nil, ValidationError{Field: "name", Message: "name is required"}
	}
	if in.Code != nil && *in.Code <= 0 {
		return nil, ValidationError{Field: "code", Message: "code must be greater than zero"}
	}
	if in.Price < 0 {
		return nil, ValidationError{Field: "price", Message: "price must not be negative"}
	}
	if err := validateEstimatedTime(in.EstimatedTime); err != nil {
		return nil, err
	}
	return c.store.Create(ctx, in)
}

func (c *Catalog) List(ctx context.Context, filter ListFilter) ([]Service, error) {
	return c.store.List(ctx, filter)
}

func (c *Catalog) ByID(ctx context.Context, id string) (*Service, error) {
	return c.store.FindByID(ctx, id)
}

func (c *Catalog) Update(ctx context.Context, id string, changes Changes) (*Service, error) {
	if changes.Name != nil {
		name := strings.TrimSpace(*changes.Name)
		if name == "" {
			return nil, ValidationError{Field: "name", Message: "name must not be empty"}
		}
		changes.Name = &name
	}
	if changes.Description != nil {
		description := strings.TrimSpace(*changes.Description)
		changes.Description = &description
	}
	if changes.Price != nil && *changes.Price < 0 {
		return nil, ValidationError{Field: "price", Message: "price must not be negative"}
	}
	if err := validateEstimatedTime(changes.EstimatedTime); err != nil {
		return nil, err
	}
	if changes.Name == nil && changes.Description == nil && changes.Price == nil && changes.EstimatedTime == nil {
		return nil, ValidationError{Message: "at least one of name, description, price or estimated_time is required"}
	}
	return c.store.Update(ctx, id, changes)
}

// Deactivate is the logical deletion exposed by DELETE (FR5, BR7).
func (c *Catalog) Deactivate(ctx context.Context, id string) error {
	return c.store.Deactivate(ctx, id)
}

func validateEstimatedTime(estimatedTime *int) error {
	if estimatedTime != nil && *estimatedTime <= 0 {
		return ValidationError{Field: "estimated_time", Message: "estimated_time must be greater than zero"}
	}
	return nil
}
