package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/shared/document"
)

// CreateInput carries the raw fields a create request supplies. Exactly one
// of CustomerID/CustomerDocument and exactly one of VehicleID/LicensePlate
// are expected to be non-empty — the HTTP adapter's CreateRequest.Validate()
// enforces that shape before this ever runs (design.md §1.5), so Create only
// has to pick whichever one was given.
type CreateInput struct {
	CustomerID          string
	CustomerDocument    string
	VehicleID           string
	LicensePlate        string
	RequestedServiceIDs []string
	RequestedProducts   []RequestedProductInput
	Notes               string
}

// RequestedProductInput is a raw, unresolved requested product as supplied
// by a create request (specs/service-order-opening/design.md §7.4).
type RequestedProductInput struct {
	ProductID string
	Quantity  int
}

// CreateResult bundles the created aggregate with the display data
// (customer, vehicle, requested services) the response needs, so the
// handler does not have to make its own follow-up repository calls
// (design.md §4.2).
type CreateResult struct {
	Order         *domain.ServiceOrder
	Customer      *CustomerRef
	Vehicle       *VehicleRef
	Services      []*ServiceRef
	Products      []*RequestedProductRef
	TrackingToken string
}

// RequestedProductRef pairs a requested product's catalog display data with
// the quantity asked for — the quantity comes from the request, not from a
// query (design.md §7.4).
type RequestedProductRef struct {
	Product  *ProductRef
	Quantity int
}

// Opening is the Service Order Opening use case
// (specs/service-order-opening/).
type Opening struct {
	orders    OrderCreator
	customers CustomerLookup
	vehicles  VehicleLookup
	catalog   ServiceCatalogLookup
	products  ProductLookup
}

func NewOpening(orders OrderCreator, customers CustomerLookup, vehicles VehicleLookup, catalog ServiceCatalogLookup, products ProductLookup) *Opening {
	return &Opening{orders: orders, customers: customers, vehicles: vehicles, catalog: catalog, products: products}
}

// Create resolves the customer and vehicle referenced by input, validates
// every business rule from requirements.md §3, and persists the new
// service order transactionally.
func (opening *Opening) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	customer, err := opening.resolveCustomer(ctx, input)
	if err != nil {
		return nil, err
	}

	vehicle, err := opening.resolveVehicle(ctx, input)
	if err != nil {
		return nil, err
	}
	if vehicle.CustomerID != customer.ID {
		return nil, domain.ErrVehicleNotOwnedByCustomer
	}

	requestedServiceIDs, err := parseUUIDs(input.RequestedServiceIDs)
	if err != nil {
		return nil, err
	}
	if len(requestedServiceIDs) > 0 {
		missing, err := opening.catalog.FindMissingServiceIDs(ctx, requestedServiceIDs)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			return nil, domain.ErrRequestedServiceNotFound
		}
	}

	requestedProducts, products, err := opening.resolveRequestedProducts(ctx, input.RequestedProducts)
	if err != nil {
		return nil, err
	}

	order, err := domain.NewServiceOrder(customer.ID, vehicle.ID, input.Notes, requestedServiceIDs, requestedProducts)
	if err != nil {
		return nil, err
	}

	trackingToken, err := opening.orders.Create(ctx, order)
	if err != nil {
		return nil, err
	}

	requestedServices, err := opening.catalog.FindServicesByIDs(ctx, requestedServiceIDs)
	if err != nil {
		return nil, err
	}

	return &CreateResult{
		Order:         order,
		Customer:      customer,
		Vehicle:       vehicle,
		Services:      requestedServices,
		Products:      products,
		TrackingToken: trackingToken,
	}, nil
}

// resolveRequestedProducts validates every requested product
// (specs/service-order-opening/requirements.md §7a): positive quantity, no
// repeated product id, and an existing ACTIVE catalog product. Structural
// checks run first, over the whole list, so a malformed request never
// reaches the database. No stock balance is read or checked here (§7a.1).
// It reuses FindActiveProductByID, the quote feature's lookup (design.md
// §7.2), and keeps its result as the response's display data.
func (opening *Opening) resolveRequestedProducts(ctx context.Context, inputs []RequestedProductInput) ([]domain.RequestedProduct, []*RequestedProductRef, error) {
	if len(inputs) == 0 {
		return nil, nil, nil
	}

	requested := make([]domain.RequestedProduct, len(inputs))
	seen := make(map[uuid.UUID]bool, len(inputs))
	for index, input := range inputs {
		id, err := uuid.Parse(input.ProductID)
		if err != nil {
			return nil, nil, domain.ErrRequestedProductNotFound
		}
		if input.Quantity <= 0 {
			return nil, nil, domain.ErrInvalidRequestedProductQuantity
		}
		if seen[id] {
			return nil, nil, domain.ErrDuplicateRequestedProduct
		}
		seen[id] = true
		requested[index] = domain.RequestedProduct{ProductID: id, Quantity: input.Quantity}
	}

	refs := make([]*RequestedProductRef, len(requested))
	for index, product := range requested {
		ref, err := opening.products.FindActiveProductByID(ctx, product.ProductID)
		if err != nil {
			if errors.Is(err, domain.ErrProductNotFound) {
				return nil, nil, domain.ErrRequestedProductNotFound
			}
			return nil, nil, err
		}
		if !ref.Active {
			return nil, nil, domain.ErrRequestedProductNotFound
		}
		refs[index] = &RequestedProductRef{Product: ref, Quantity: product.Quantity}
	}
	return requested, refs, nil
}

func (opening *Opening) resolveCustomer(ctx context.Context, input CreateInput) (*CustomerRef, error) {
	var (
		customer *CustomerRef
		err      error
	)
	if input.CustomerID != "" {
		id, parseErr := uuid.Parse(input.CustomerID)
		if parseErr != nil {
			return nil, domain.ErrCustomerNotFound
		}
		customer, err = opening.customers.FindCustomerByID(ctx, id)
	} else {
		customer, err = opening.customers.FindCustomerByDocument(ctx, document.Normalize(input.CustomerDocument))
	}
	if err != nil {
		return nil, err
	}
	if !customer.Active {
		return nil, domain.ErrCustomerInactive
	}
	return customer, nil
}

func (opening *Opening) resolveVehicle(ctx context.Context, input CreateInput) (*VehicleRef, error) {
	var (
		vehicle *VehicleRef
		err     error
	)
	if input.VehicleID != "" {
		id, parseErr := uuid.Parse(input.VehicleID)
		if parseErr != nil {
			return nil, domain.ErrVehicleNotFound
		}
		vehicle, err = opening.vehicles.FindVehicleByID(ctx, id)
	} else {
		vehicle, err = opening.vehicles.FindVehicleByPlate(ctx, input.LicensePlate)
	}
	if err != nil {
		return nil, err
	}
	if !vehicle.Active {
		return nil, domain.ErrVehicleInactive
	}
	return vehicle, nil
}

// parseUUIDs parses every raw id in values. An unparseable id is reported as
// domain.ErrRequestedServiceNotFound — from the caller's perspective a
// malformed id can never match a real service, same observable outcome as a
// well-formed but unknown id (requirements.md §3.7 doesn't distinguish the
// two cases).
func parseUUIDs(values []string) ([]uuid.UUID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(values))
	for index, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, domain.ErrRequestedServiceNotFound
		}
		ids[index] = id
	}
	return ids, nil
}
