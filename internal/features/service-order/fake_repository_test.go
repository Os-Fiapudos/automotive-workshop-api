package serviceorder

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// fakeRepository is an in-memory ServiceOrderRepository + serviceOrderLookups
// used only by the service-level unit tests in this package — no mocking
// framework, same convention as internal/features/customer/fake_repository_test.go.
type fakeRepository struct {
	orders            []*ServiceOrder
	customers         map[uuid.UUID]*customerRef
	customersByDoc    map[string]*customerRef
	vehicles          map[uuid.UUID]*vehicleRef
	services          map[uuid.UUID]*serviceRef
	products          map[uuid.UUID]*productRef
	quotes            map[uuid.UUID]*Quote                 // keyed by service order id
	requestedServices map[uuid.UUID][]*serviceRef          // keyed by service order id
	history           map[uuid.UUID][]*ServiceOrderHistory // keyed by service order id
	executions        map[uuid.UUID][]*ServiceExecution    // keyed by service order id
	trackingTokens    map[uuid.UUID]string                 // keyed by service order id, value is the token hash
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		customers:         make(map[uuid.UUID]*customerRef),
		customersByDoc:    make(map[string]*customerRef),
		vehicles:          make(map[uuid.UUID]*vehicleRef),
		services:          make(map[uuid.UUID]*serviceRef),
		products:          make(map[uuid.UUID]*productRef),
		quotes:            make(map[uuid.UUID]*Quote),
		requestedServices: make(map[uuid.UUID][]*serviceRef),
		history:           make(map[uuid.UUID][]*ServiceOrderHistory),
		executions:        make(map[uuid.UUID][]*ServiceExecution),
		trackingTokens:    make(map[uuid.UUID]string),
	}
}

// addTrackingToken registers orderID's tracking token hash, so
// findServiceOrderByCodeWithTrackingToken can validate it — used by
// quote_service_test.go's ApproveQuote/RejectQuote tests
// (specs/service-order-quote-decision/).
func (fake *fakeRepository) addTrackingToken(orderID uuid.UUID, tokenHash string) {
	fake.trackingTokens[orderID] = tokenHash
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

func (fake *fakeRepository) Create(_ context.Context, order *ServiceOrder) (string, error) {
	order.Code = int64(len(fake.orders) + 1)
	fake.orders = append(fake.orders, order)
	rawToken, err := trackingtoken.Generate()
	if err != nil {
		return "", err
	}
	return rawToken, nil
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
		quote = &Quote{ID: uuid.New(), ServiceOrderID: order.ID, Version: 1}
	} else {
		quote.Version++
	}
	quote.TotalAmount = total
	quote.Status = QuoteStatusPending
	quote.Items = items
	fake.quotes[order.ID] = quote
	return quote, nil
}

// SendQuote implements ServiceOrderRepository
// (specs/service-order-quote-decision/) for the service-level unit tests.
func (fake *fakeRepository) SendQuote(_ context.Context, _ *ServiceOrder, quote *Quote) (*Quote, error) {
	now := time.Now().UTC()
	quote.SentAt = &now
	sentVersion := quote.Version
	quote.SentVersion = &sentVersion
	return quote, nil
}

// DecideQuote implements ServiceOrderRepository
// (specs/service-order-quote-decision/) for the service-level unit tests.
func (fake *fakeRepository) DecideQuote(_ context.Context, _ *ServiceOrder, quote *Quote, decision QuoteStatus) (*Quote, error) {
	if quote.Status != QuoteStatusPending {
		return nil, ErrQuoteAlreadyDecided
	}
	now := time.Now().UTC()
	quote.Status = decision
	quote.RespondedAt = &now
	return quote, nil
}

// findServiceOrderByCodeWithTrackingToken implements serviceOrderLookups
// (specs/service-order-quote-decision/) for the service-level unit tests.
func (fake *fakeRepository) findServiceOrderByCodeWithTrackingToken(_ context.Context, code int64, tokenHash string) (*ServiceOrder, error) {
	for _, order := range fake.orders {
		if order.Code != code {
			continue
		}
		if fake.trackingTokens[order.ID] != tokenHash {
			return nil, ErrTrackingTokenInvalid
		}
		return order, nil
	}
	return nil, ErrServiceOrderNotFound
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

// setRequestedServices/addHistory seed the query-only projections
// findRequestedServices/findHistoryByServiceOrderID read back, used by
// query_service_test.go (specs/service-order-query/).
func (fake *fakeRepository) setRequestedServices(orderID uuid.UUID, refs []*serviceRef) {
	fake.requestedServices[orderID] = refs
}

func (fake *fakeRepository) addHistory(orderID uuid.UUID, entry *ServiceOrderHistory) {
	fake.history[orderID] = append(fake.history[orderID], entry)
}

func (fake *fakeRepository) findServiceOrderByCode(_ context.Context, code int64) (*ServiceOrder, error) {
	for _, order := range fake.orders {
		if order.Code == code {
			return order, nil
		}
	}
	return nil, ErrServiceOrderNotFound
}

func (fake *fakeRepository) findRequestedServices(_ context.Context, serviceOrderID uuid.UUID) ([]*serviceRef, error) {
	return fake.requestedServices[serviceOrderID], nil
}

func (fake *fakeRepository) findHistoryByServiceOrderID(_ context.Context, serviceOrderID uuid.UUID) ([]*ServiceOrderHistory, error) {
	return fake.history[serviceOrderID], nil
}

// ---- specs/service-order-execution/ ----

func (fake *fakeRepository) StartExecution(_ context.Context, execution *ServiceExecution) error {
	execution.StartedAt = time.Now().UTC()
	fake.executions[execution.ServiceOrderID] = append(fake.executions[execution.ServiceOrderID], execution)
	return nil
}

func (fake *fakeRepository) FinishExecution(_ context.Context, execution *ServiceExecution) error {
	for _, existing := range fake.executions[execution.ServiceOrderID] {
		if existing.ID == execution.ID {
			endedAt := execution.EndedAt
			if endedAt == nil {
				now := time.Now().UTC()
				endedAt = &now
			}
			existing.EndedAt = endedAt
			execution.EndedAt = endedAt
			return nil
		}
	}
	return ErrServiceExecutionNotFound
}

func (fake *fakeRepository) FinalizeOrder(_ context.Context, order *ServiceOrder) error {
	return nil
}

func (fake *fakeRepository) DeliverOrder(_ context.Context, order *ServiceOrder) error {
	return nil
}

func (fake *fakeRepository) findServiceExecutionByID(_ context.Context, serviceOrderID, executionID uuid.UUID) (*ServiceExecution, error) {
	for _, execution := range fake.executions[serviceOrderID] {
		if execution.ID == executionID {
			return execution, nil
		}
	}
	return nil, ErrServiceExecutionNotFound
}

func (fake *fakeRepository) findServiceExecutionsByServiceOrderID(_ context.Context, serviceOrderID uuid.UUID) ([]*ServiceExecution, error) {
	return fake.executions[serviceOrderID], nil
}

// listServiceOrders is an in-memory equivalent of
// PostgresServiceOrderRepository.listServiceOrders, filtering/sorting/paging
// the same way the real SQL query does (design.md §1.3/§1.4), for the
// service-layer unit tests to exercise without a database.
func (fake *fakeRepository) listServiceOrders(_ context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error) {
	var matched []*ServiceOrderListItem
	for _, order := range fake.orders {
		if filter.Code != nil && order.Code != *filter.Code {
			continue
		}
		if filter.Status != nil && string(order.Status) != *filter.Status {
			continue
		}
		customer := fake.customers[order.CustomerID]
		if filter.CustomerDocument != "" && (customer == nil || customer.Document != filter.CustomerDocument) {
			continue
		}
		vehicle := fake.vehicles[order.VehicleID]
		if filter.LicensePlate != "" && (vehicle == nil || vehicle.LicensePlate != filter.LicensePlate) {
			continue
		}
		if filter.CreatedFrom != nil && order.CreatedAt.Before(*filter.CreatedFrom) {
			continue
		}
		if filter.CreatedTo != nil && order.CreatedAt.After(*filter.CreatedTo) {
			continue
		}
		matched = append(matched, &ServiceOrderListItem{Order: order, Customer: customer, Vehicle: vehicle})
	}

	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].Order.CreatedAt.Equal(matched[j].Order.CreatedAt) {
			return matched[i].Order.CreatedAt.After(matched[j].Order.CreatedAt)
		}
		return matched[i].Order.Code > matched[j].Order.Code
	})

	total := len(matched)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
}
