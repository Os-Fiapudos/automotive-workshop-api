package serviceorder

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedListOrder builds and registers a ready-to-list order for customerID/
// vehicleID, with a deterministic CreatedAt so ordering assertions are
// stable.
func seedListOrder(repo *fakeRepository, customerID, vehicleID uuid.UUID, status Status, createdAt time.Time) *ServiceOrder {
	order := &ServiceOrder{
		ID:         uuid.New(),
		CustomerID: customerID,
		VehicleID:  vehicleID,
		Status:     status,
		OpenedAt:   createdAt,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}
	repo.addOrder(order)
	return order
}

func TestListNoFilterOrdersOldestFirstWithinStatus(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	older := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now().Add(-2*time.Hour))
	newer := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())

	items, total, err := service.List(context.Background(), ListFilter{}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, items, 2)
	assert.Equal(t, older.ID, items[0].Order.ID)
	assert.Equal(t, newer.ID, items[1].Order.ID)
}

// TestListNoFilterOrdersByStatusPriority covers requirements.md's Fase 2
// addendum to BR3/AC1: with no status filter, orders are grouped
// IN_PROGRESS > AWAITING_APPROVAL > IN_DIAGNOSIS > RECEIVED regardless of
// creation order, and COMPLETED/DELIVERED/CANCELED are excluded entirely.
func TestListNoFilterOrdersByStatusPriority(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	now := time.Now()
	received := seedListOrder(repo, customerID, vehicleID, StatusReceived, now.Add(-10*time.Minute))
	inDiagnosis := seedListOrder(repo, customerID, vehicleID, StatusInDiagnosis, now.Add(-9*time.Minute))
	awaitingApproval := seedListOrder(repo, customerID, vehicleID, StatusAwaitingApproval, now.Add(-8*time.Minute))
	inProgress := seedListOrder(repo, customerID, vehicleID, StatusInProgress, now.Add(-7*time.Minute))
	seedListOrder(repo, customerID, vehicleID, StatusCompleted, now.Add(-6*time.Minute))
	seedListOrder(repo, customerID, vehicleID, StatusDelivered, now.Add(-5*time.Minute))
	seedListOrder(repo, customerID, vehicleID, StatusCanceled, now.Add(-4*time.Minute))

	items, total, err := service.List(context.Background(), ListFilter{}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	require.Len(t, items, 4)
	assert.Equal(t, inProgress.ID, items[0].Order.ID)
	assert.Equal(t, awaitingApproval.ID, items[1].Order.ID)
	assert.Equal(t, inDiagnosis.ID, items[2].Order.ID)
	assert.Equal(t, received.ID, items[3].Order.ID)
}

// TestListExplicitTerminalStatusFilterBypassesDefaultExclusion covers
// requirements.md AC12: status=COMPLETED/DELIVERED/CANCELED still returns
// those orders, ordered oldest first, even though the default view excludes
// them.
func TestListExplicitTerminalStatusFilterBypassesDefaultExclusion(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	now := time.Now()
	olderCompleted := seedListOrder(repo, customerID, vehicleID, StatusCompleted, now.Add(-2*time.Hour))
	newerCompleted := seedListOrder(repo, customerID, vehicleID, StatusCompleted, now)
	seedListOrder(repo, customerID, vehicleID, StatusDelivered, now)
	seedListOrder(repo, customerID, vehicleID, StatusCanceled, now)

	status := string(StatusCompleted)
	items, total, err := service.List(context.Background(), ListFilter{Status: &status}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, items, 2)
	assert.Equal(t, olderCompleted.ID, items[0].Order.ID)
	assert.Equal(t, newerCompleted.ID, items[1].Order.ID)
}

func TestListFiltersByCode(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())
	target := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())

	items, total, err := service.List(context.Background(), ListFilter{Code: &target.Code}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, target.ID, items[0].Order.ID)
}

func TestListFiltersByStatus(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())
	diagnosing := seedListOrder(repo, customerID, vehicleID, StatusInDiagnosis, time.Now())

	status := string(StatusInDiagnosis)
	items, total, err := service.List(context.Background(), ListFilter{Status: &status}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, diagnosing.ID, items[0].Order.ID)
}

func TestListFiltersByCustomerDocument(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	target := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())

	otherCustomerID := uuid.New()
	otherVehicleID := uuid.New()
	repo.addCustomer(&customerRef{ID: otherCustomerID, Code: 2, Name: "Other", Active: true, Document: "22233344400"}, "22233344400")
	repo.addVehicle(&vehicleRef{ID: otherVehicleID, Code: 2, LicensePlate: "XYZ9Z99", CustomerID: otherCustomerID, Active: true})
	seedListOrder(repo, otherCustomerID, otherVehicleID, StatusReceived, time.Now())

	items, total, err := service.List(context.Background(), ListFilter{CustomerDocument: normalizedDocument}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, target.ID, items[0].Order.ID)
}

func TestListFiltersByLicensePlate(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	target := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())

	otherVehicleID := uuid.New()
	repo.addVehicle(&vehicleRef{ID: otherVehicleID, Code: 2, LicensePlate: "XYZ9Z99", CustomerID: customerID, Active: true})
	seedListOrder(repo, customerID, otherVehicleID, StatusReceived, time.Now())

	items, total, err := service.List(context.Background(), ListFilter{LicensePlate: "ABC1D23"}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, target.ID, items[0].Order.ID)
}

func TestListFiltersByCreatedRange(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	base := time.Now()
	seedListOrder(repo, customerID, vehicleID, StatusReceived, base.Add(-48*time.Hour))
	inRange := seedListOrder(repo, customerID, vehicleID, StatusReceived, base.Add(-24*time.Hour))
	seedListOrder(repo, customerID, vehicleID, StatusReceived, base)

	from := base.Add(-30 * time.Hour)
	to := base.Add(-1 * time.Hour)
	items, total, err := service.List(context.Background(), ListFilter{CreatedFrom: &from, CreatedTo: &to}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, inRange.ID, items[0].Order.ID)
}

func TestListCombinesFiltersWithAnd(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())
	target := seedListOrder(repo, customerID, vehicleID, StatusInDiagnosis, time.Now())

	status := string(StatusInDiagnosis)
	items, total, err := service.List(context.Background(), ListFilter{
		Status:           &status,
		CustomerDocument: normalizedDocument,
	}, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, target.ID, items[0].Order.ID)
}

func TestListPagination(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	for i := 0; i < 5; i++ {
		seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now().Add(time.Duration(i)*time.Minute))
	}

	firstPage, total, err := service.List(context.Background(), ListFilter{}, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, firstPage, 2)

	lastPage, total, err := service.List(context.Background(), ListFilter{}, 3, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, lastPage, 1)
}

func TestGetDetailAssemblesEveryField(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	order := seedListOrder(repo, customerID, vehicleID, StatusAwaitingApproval, time.Now())

	requestedServiceID := uuid.New()
	repo.setRequestedServices(order.ID, []*serviceRef{{ID: requestedServiceID, Code: 1, Name: "Oil Change"}})
	repo.addHistory(order.ID, &ServiceOrderHistory{
		ID: uuid.New(), ServiceOrderID: order.ID, Event: "creation",
		PreviousStatus: StatusReceived, NewStatus: StatusReceived,
	})
	repo.addHistory(order.ID, &ServiceOrderHistory{
		ID: uuid.New(), ServiceOrderID: order.ID, Event: "quote_composed",
		PreviousStatus: StatusInDiagnosis, NewStatus: StatusAwaitingApproval,
	})
	repo.quotes[order.ID] = &Quote{ID: uuid.New(), ServiceOrderID: order.ID, Status: QuoteStatusPending, TotalAmount: 150}

	detail, err := service.GetDetail(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, order.ID, detail.Order.ID)
	assert.Equal(t, customerID, detail.Customer.ID)
	assert.Equal(t, vehicleID, detail.Vehicle.ID)
	require.Len(t, detail.RequestedServices, 1)
	assert.Equal(t, requestedServiceID, detail.RequestedServices[0].ID)
	require.Len(t, detail.History, 2)
	require.NotNil(t, detail.Quote)
	assert.Equal(t, 150.0, detail.Quote.TotalAmount)
}

func TestGetDetailQuoteNilBeforeComposition(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	order := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())

	detail, err := service.GetDetail(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Nil(t, detail.Quote)
}

func TestGetDetailNotFound(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.GetDetail(context.Background(), uuid.New())
	assert.ErrorIs(t, err, ErrServiceOrderNotFound)
}

func TestGetDetailByCode(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	order := seedListOrder(repo, customerID, vehicleID, StatusReceived, time.Now())
	order.Code = 42

	detail, err := service.GetDetailByCode(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, order.ID, detail.Order.ID)
}

func TestGetDetailByCodeNotFound(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.GetDetailByCode(context.Background(), 9999)
	assert.ErrorIs(t, err, ErrServiceOrderNotFound)
}
