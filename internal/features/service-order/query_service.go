package serviceorder

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
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
	Order    *ServiceOrder
	Customer *customerRef
	Vehicle  *vehicleRef
}

// ServiceOrderDetail is the full read model for a single service order
// (design.md §1.8). Quote is nil when no quote has been composed yet.
type ServiceOrderDetail struct {
	Order             *ServiceOrder
	Customer          *customerRef
	Vehicle           *vehicleRef
	RequestedServices []*serviceRef
	Quote             *Quote
	History           []*ServiceOrderHistory
}

// List retrieves a filtered, paginated page of service orders, most recent
// first (requirements.md BR1-BR3).
func (service *ServiceOrderService) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error) {
	return service.lookups.listServiceOrders(ctx, filter, page, pageSize)
}

// GetDetail retrieves the full detail view of a service order by its
// technical id (requirements.md BR4).
func (service *ServiceOrderService) GetDetail(ctx context.Context, id uuid.UUID) (*ServiceOrderDetail, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return service.buildDetail(ctx, order)
}

// GetDetailByCode retrieves the full detail view of a service order by its
// sequential code (requirements.md BR4).
func (service *ServiceOrderService) GetDetailByCode(ctx context.Context, code int64) (*ServiceOrderDetail, error) {
	order, err := service.lookups.findServiceOrderByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return service.buildDetail(ctx, order)
}

// buildDetail assembles a ServiceOrderDetail once the order itself is
// resolved, shared by GetDetail/GetDetailByCode (design.md §1.8). It never
// filters any related record by status/active (requirements.md BR6) — an
// inactive customer/vehicle or an order with no quote yet are both valid,
// fully-displayed states, not errors.
func (service *ServiceOrderService) buildDetail(ctx context.Context, order *ServiceOrder) (*ServiceOrderDetail, error) {
	customer, err := service.lookups.findCustomerByID(ctx, order.CustomerID)
	if err != nil {
		return nil, err
	}

	vehicle, err := service.lookups.findVehicleByID(ctx, order.VehicleID)
	if err != nil {
		return nil, err
	}

	requestedServices, err := service.lookups.findRequestedServices(ctx, order.ID)
	if err != nil {
		return nil, err
	}

	history, err := service.lookups.findHistoryByServiceOrderID(ctx, order.ID)
	if err != nil {
		return nil, err
	}

	quote, err := service.repository.FindQuoteByServiceOrderID(ctx, order.ID)
	if err != nil && !errors.Is(err, ErrQuoteNotFound) {
		return nil, err
	}
	if errors.Is(err, ErrQuoteNotFound) {
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
