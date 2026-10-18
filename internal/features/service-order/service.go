package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/document"
)

// CreateInput carries the raw fields a create request supplies. Exactly one
// of CustomerID/CustomerDocument and exactly one of VehicleID/LicensePlate
// are expected to be non-empty — dto.CreateRequest.Validate() enforces that
// shape before this ever runs (design.md §1.5), so Create only has to pick
// whichever one was given.
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
	Order         *ServiceOrder
	Customer      *customerRef
	Vehicle       *vehicleRef
	Services      []*serviceRef
	Products      []*requestedProductRef
	TrackingToken string
}

// requestedProductRef pairs a requested product's catalog display data with
// the quantity asked for — the quantity comes from the request, not from a
// query (design.md §7.4).
type requestedProductRef struct {
	Product  *productRef
	Quantity int
}

// ServiceOrderService orchestrates every use case on the ServiceOrder
// aggregate on top of a ServiceOrderRepository and its read-only lookups.
// notifier is the quote-sent notification port
// (specs/service-order-quote-decision/, see quote_notifier.go) — always
// non-nil; NewServiceOrderService defaults it to NoOpQuoteNotifier when nil
// is passed, so existing callers (and tests) that only care about the other
// use cases don't have to wire one explicitly.
// externalNotificationSecret authenticates the external quote-decision
// channel (specs/service-order-quote-decision/design.md §5) — set once at
// wiring time via WithExternalNotificationSecret; left empty, that channel
// rejects every caller.
type ServiceOrderService struct {
	repository                 ServiceOrderRepository
	lookups                    serviceOrderLookups
	notifier                   QuoteNotifier
	externalNotificationSecret string
}

func NewServiceOrderService(repository ServiceOrderRepository, lookups serviceOrderLookups, notifier QuoteNotifier) *ServiceOrderService {
	if notifier == nil {
		notifier = NoOpQuoteNotifier{}
	}
	return &ServiceOrderService{repository: repository, lookups: lookups, notifier: notifier}
}

// WithExternalNotificationSecret sets the shared secret the external
// quote-decision endpoint authenticates against (design.md §5.4) and returns
// the same service, so it chains onto NewServiceOrderService at wiring time
// without changing that constructor's signature for every existing caller.
func (service *ServiceOrderService) WithExternalNotificationSecret(secret string) *ServiceOrderService {
	service.externalNotificationSecret = secret
	return service
}

// Create resolves the customer and vehicle referenced by input, validates
// every business rule from requirements.md §3, and persists the new
// service order transactionally.
func (service *ServiceOrderService) Create(ctx context.Context, input CreateInput) (*CreateResult, error) {
	customer, err := service.resolveCustomer(ctx, input)
	if err != nil {
		return nil, err
	}

	vehicle, err := service.resolveVehicle(ctx, input)
	if err != nil {
		return nil, err
	}
	if vehicle.CustomerID != customer.ID {
		return nil, ErrVehicleNotOwnedByCustomer
	}

	requestedServiceIDs, err := parseUUIDs(input.RequestedServiceIDs)
	if err != nil {
		return nil, err
	}
	if len(requestedServiceIDs) > 0 {
		missing, err := service.lookups.findMissingServiceIDs(ctx, requestedServiceIDs)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			return nil, ErrRequestedServiceNotFound
		}
	}

	requestedProducts, products, err := service.resolveRequestedProducts(ctx, input.RequestedProducts)
	if err != nil {
		return nil, err
	}

	order, err := NewServiceOrder(customer.ID, vehicle.ID, input.Notes, requestedServiceIDs, requestedProducts)
	if err != nil {
		return nil, err
	}

	trackingToken, err := service.repository.Create(ctx, order)
	if err != nil {
		return nil, err
	}

	requestedServices, err := service.lookups.findServicesByIDs(ctx, requestedServiceIDs)
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
// It reuses findActiveProductByID, the quote feature's lookup (design.md
// §7.2), and keeps its result as the response's display data.
func (service *ServiceOrderService) resolveRequestedProducts(ctx context.Context, inputs []RequestedProductInput) ([]RequestedProduct, []*requestedProductRef, error) {
	if len(inputs) == 0 {
		return nil, nil, nil
	}

	requested := make([]RequestedProduct, len(inputs))
	seen := make(map[uuid.UUID]bool, len(inputs))
	for index, input := range inputs {
		id, err := uuid.Parse(input.ProductID)
		if err != nil {
			return nil, nil, ErrRequestedProductNotFound
		}
		if input.Quantity <= 0 {
			return nil, nil, ErrInvalidRequestedProductQuantity
		}
		if seen[id] {
			return nil, nil, ErrDuplicateRequestedProduct
		}
		seen[id] = true
		requested[index] = RequestedProduct{ProductID: id, Quantity: input.Quantity}
	}

	refs := make([]*requestedProductRef, len(requested))
	for index, product := range requested {
		ref, err := service.lookups.findActiveProductByID(ctx, product.ProductID)
		if err != nil {
			if errors.Is(err, ErrProductNotFound) {
				return nil, nil, ErrRequestedProductNotFound
			}
			return nil, nil, err
		}
		if !ref.Active {
			return nil, nil, ErrRequestedProductNotFound
		}
		refs[index] = &requestedProductRef{Product: ref, Quantity: product.Quantity}
	}
	return requested, refs, nil
}

func (service *ServiceOrderService) resolveCustomer(ctx context.Context, input CreateInput) (*customerRef, error) {
	var (
		customer *customerRef
		err      error
	)
	if input.CustomerID != "" {
		id, parseErr := uuid.Parse(input.CustomerID)
		if parseErr != nil {
			return nil, ErrCustomerNotFound
		}
		customer, err = service.lookups.findCustomerByID(ctx, id)
	} else {
		customer, err = service.lookups.findCustomerByDocument(ctx, document.Normalize(input.CustomerDocument))
	}
	if err != nil {
		return nil, err
	}
	if !customer.Active {
		return nil, ErrCustomerInactive
	}
	return customer, nil
}

func (service *ServiceOrderService) resolveVehicle(ctx context.Context, input CreateInput) (*vehicleRef, error) {
	var (
		vehicle *vehicleRef
		err     error
	)
	if input.VehicleID != "" {
		id, parseErr := uuid.Parse(input.VehicleID)
		if parseErr != nil {
			return nil, ErrVehicleNotFound
		}
		vehicle, err = service.lookups.findVehicleByID(ctx, id)
	} else {
		vehicle, err = service.lookups.findVehicleByPlate(ctx, input.LicensePlate)
	}
	if err != nil {
		return nil, err
	}
	if !vehicle.Active {
		return nil, ErrVehicleInactive
	}
	return vehicle, nil
}

// parseUUIDs parses every raw id in values. An unparseable id is reported as
// ErrRequestedServiceNotFound — from the caller's perspective a malformed id
// can never match a real service, same observable outcome as a well-formed
// but unknown id (requirements.md §3.7 doesn't distinguish the two cases).
func parseUUIDs(values []string) ([]uuid.UUID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(values))
	for index, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, ErrRequestedServiceNotFound
		}
		ids[index] = id
	}
	return ids, nil
}
