package serviceorder

import (
	"context"

	"github.com/google/uuid"
)

// fakeRepository is an in-memory ServiceOrderRepository + serviceOrderLookups
// used only by the service-level unit tests in this package — no mocking
// framework, same convention as internal/features/customer/fake_repository_test.go.
type fakeRepository struct {
	orders         []*ServiceOrder
	customers      map[uuid.UUID]*customerRef
	customersByDoc map[string]*customerRef
	vehicles       map[uuid.UUID]*vehicleRef
	services       map[uuid.UUID]*serviceRef
	products       map[uuid.UUID]*productRef
	quotes         map[uuid.UUID]*Quote // keyed by service order id
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		customers:      make(map[uuid.UUID]*customerRef),
		customersByDoc: make(map[string]*customerRef),
		vehicles:       make(map[uuid.UUID]*vehicleRef),
		services:       make(map[uuid.UUID]*serviceRef),
		products:       make(map[uuid.UUID]*productRef),
		quotes:         make(map[uuid.UUID]*Quote),
	}
}

func (fake *fakeRepository) addCustomer(ref *customerRef, document string) {
	fake.customers[ref.ID] = ref
	fake.customersByDoc[document] = ref
}
func (fake *fakeRepository) addVehicle(ref *vehicleRef) { fake.vehicles[ref.ID] = ref }
func (fake *fakeRepository) addService(ref *serviceRef) { fake.services[ref.ID] = ref }
func (fake *fakeRepository) addProduct(ref *productRef) { fake.products[ref.ID] = ref }

// addOrder registers an already-built order so quote_service_test.go can
// exercise StartDiagnosis/ComposeQuote without going through Create.
func (fake *fakeRepository) addOrder(order *ServiceOrder) {
	if order.Code == 0 {
		order.Code = int64(len(fake.orders) + 1)
	}
	fake.orders = append(fake.orders, order)
}

// seedDecidedQuote registers a quote already APPROVED/REJECTED for order,
// so tests can exercise ErrQuoteAlreadyDecided without a real compose call.
func (fake *fakeRepository) seedDecidedQuote(orderID uuid.UUID, status QuoteStatus) {
	fake.quotes[orderID] = &Quote{ID: uuid.New(), ServiceOrderID: orderID, Status: status}
}

func (fake *fakeRepository) Create(_ context.Context, order *ServiceOrder) error {
	order.Code = int64(len(fake.orders) + 1)
	fake.orders = append(fake.orders, order)
	return nil
}

func (fake *fakeRepository) findServiceOrderByID(_ context.Context, id uuid.UUID) (*ServiceOrder, error) {
	for _, order := range fake.orders {
		if order.ID == id {
			return order, nil
		}
	}
	return nil, ErrServiceOrderNotFound
}

func (fake *fakeRepository) findActiveProductByID(_ context.Context, id uuid.UUID) (*productRef, error) {
	ref, ok := fake.products[id]
	if !ok {
		return nil, ErrProductNotFound
	}
	return ref, nil
}

func (fake *fakeRepository) findServiceByID(_ context.Context, id uuid.UUID) (*serviceRef, error) {
	ref, ok := fake.services[id]
	if !ok {
		return nil, ErrRequestedServiceNotFound
	}
	return ref, nil
}

func (fake *fakeRepository) StartDiagnosis(_ context.Context, order *ServiceOrder) error {
	return nil
}

func (fake *fakeRepository) SaveQuote(_ context.Context, order *ServiceOrder, items []QuoteItem, total float64) (*Quote, error) {
	quote := fake.quotes[order.ID]
	if quote == nil {
		quote = &Quote{ID: uuid.New(), ServiceOrderID: order.ID}
	}
	quote.TotalAmount = total
	quote.Status = QuoteStatusPending
	quote.Items = items
	fake.quotes[order.ID] = quote
	return quote, nil
}

func (fake *fakeRepository) FindQuoteByServiceOrderID(_ context.Context, serviceOrderID uuid.UUID) (*Quote, error) {
	quote, ok := fake.quotes[serviceOrderID]
	if !ok {
		return nil, ErrQuoteNotFound
	}
	return quote, nil
}

func (fake *fakeRepository) findCustomerByID(_ context.Context, id uuid.UUID) (*customerRef, error) {
	ref, ok := fake.customers[id]
	if !ok {
		return nil, ErrCustomerNotFound
	}
	return ref, nil
}

func (fake *fakeRepository) findCustomerByDocument(_ context.Context, normalizedDocument string) (*customerRef, error) {
	ref, ok := fake.customersByDoc[normalizedDocument]
	if !ok {
		return nil, ErrCustomerNotFound
	}
	return ref, nil
}

func (fake *fakeRepository) findVehicleByID(_ context.Context, id uuid.UUID) (*vehicleRef, error) {
	ref, ok := fake.vehicles[id]
	if !ok {
		return nil, ErrVehicleNotFound
	}
	return ref, nil
}

func (fake *fakeRepository) findVehicleByPlate(_ context.Context, plate string) (*vehicleRef, error) {
	for _, ref := range fake.vehicles {
		if ref.LicensePlate == plate {
			return ref, nil
		}
	}
	return nil, ErrVehicleNotFound
}

func (fake *fakeRepository) findMissingServiceIDs(_ context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	var missing []uuid.UUID
	for _, id := range ids {
		if _, ok := fake.services[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func (fake *fakeRepository) findServicesByIDs(_ context.Context, ids []uuid.UUID) ([]*serviceRef, error) {
	refs := make([]*serviceRef, 0, len(ids))
	for _, id := range ids {
		if ref, ok := fake.services[id]; ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
