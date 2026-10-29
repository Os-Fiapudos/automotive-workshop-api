package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
)

// ListFilter carries the already-parsed/validated filters accepted by
// GET /api/v1/service-orders (specs/service-order-query/design.md §1.3).
// CustomerDocument/LicensePlate arrive already normalized from the handler
// (design.md §1.9); an empty value means "no filter."
type ListFilter struct {
	Code             *int64
	Status           *string
	CustomerDocument string
	LicensePlate     string
	CreatedFrom      *time.Time
	CreatedTo        *time.Time
}

// ServiceOrderListItem is one row of a service order listing, bundling the
// order with the customer/vehicle display data the response needs
// (design.md §1.8).
type ServiceOrderListItem struct {
	Order    *domain.ServiceOrder
	Customer *CustomerRef
	Vehicle  *VehicleRef
}

// ServiceOrderDetail is the full read model for a single service order
// (design.md §1.8). Quote is nil when no quote has been composed yet.
type ServiceOrderDetail struct {
	Order             *domain.ServiceOrder
	Customer          *CustomerRef
	Vehicle           *VehicleRef
	RequestedServices []*ServiceRef
	Quote             *domain.Quote
	History           []*domain.ServiceOrderHistory
}

// Queries groups the read-only service order use cases
// (specs/service-order-query/).
type Queries struct {
	orders     OrderFinder
	codes      OrderCodeFinder
	customers  CustomerLookup
	vehicles   VehicleLookup
	projection ServiceOrderQueryStore
	quotes     QuoteReader
}

func NewQueries(orders OrderFinder, codes OrderCodeFinder, customers CustomerLookup, vehicles VehicleLookup, projection ServiceOrderQueryStore, quotes QuoteReader) *Queries {
	return &Queries{orders: orders, codes: codes, customers: customers, vehicles: vehicles, projection: projection, quotes: quotes}
}

// List retrieves a filtered, paginated page of service orders. With no
// status filter, it excludes COMPLETED/DELIVERED/CANCELED and orders by
// status priority then oldest first; with a status filter, it orders
// oldest first within that status (requirements.md BR1-BR3).
func (queries *Queries) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error) {
	return queries.projection.ListServiceOrders(ctx, filter, page, pageSize)
}

// GetDetail retrieves the full detail view of a service order by its
// technical id (requirements.md BR4).
func (queries *Queries) GetDetail(ctx context.Context, id uuid.UUID) (*ServiceOrderDetail, error) {
	order, err := queries.orders.FindServiceOrderByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return queries.buildDetail(ctx, order)
}

// GetDetailByCode retrieves the full detail view of a service order by its
// sequential code (requirements.md BR4).
func (queries *Queries) GetDetailByCode(ctx context.Context, code int64) (*ServiceOrderDetail, error) {
	order, err := queries.codes.FindServiceOrderByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return queries.buildDetail(ctx, order)
}

// buildDetail assembles a ServiceOrderDetail once the order itself is
// resolved, shared by GetDetail/GetDetailByCode (design.md §1.8). It never
// filters any related record by status/active (requirements.md BR6) — an
// inactive customer/vehicle or an order with no quote yet are both valid,
// fully-displayed states, not errors.
func (queries *Queries) buildDetail(ctx context.Context, order *domain.ServiceOrder) (*ServiceOrderDetail, error) {
	customer, err := queries.customers.FindCustomerByID(ctx, order.CustomerID)
	if err != nil {
		return nil, err
	}

	vehicle, err := queries.vehicles.FindVehicleByID(ctx, order.VehicleID)
	if err != nil {
		return nil, err
	}

	requestedServices, err := queries.projection.FindRequestedServices(ctx, order.ID)
	if err != nil {
		return nil, err
	}

	history, err := queries.projection.FindHistoryByServiceOrderID(ctx, order.ID)
	if err != nil {
		return nil, err
	}

	quote, err := queries.quotes.FindQuoteByServiceOrderID(ctx, order.ID)
	if err != nil && !errors.Is(err, domain.ErrQuoteNotFound) {
		return nil, err
	}
	if errors.Is(err, domain.ErrQuoteNotFound) {
		quote = nil
	}

	return &ServiceOrderDetail{
		Order:             order,
		Customer:          customer,
		Vehicle:           vehicle,
		RequestedServices: requestedServices,
		Quote:             quote,
		History:           history,
	}, nil
}
