package serviceorder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const normalizedDocument = "11144477735"

func newTestService(repo *fakeRepository) *ServiceOrderService {
	return NewServiceOrderService(repo, repo, nil)
}

func seedActiveCustomerAndVehicle(repo *fakeRepository) (customerID, vehicleID uuid.UUID) {
	customerID = uuid.New()
	vehicleID = uuid.New()
	repo.addCustomer(&customerRef{ID: customerID, Code: 1, Name: "Maria Silva", Active: true, Document: normalizedDocument}, normalizedDocument)
	repo.addVehicle(&vehicleRef{ID: vehicleID, Code: 1, LicensePlate: "ABC1D23", CustomerID: customerID, Active: true})
	return customerID, vehicleID
}

func TestServiceCreateByID(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})

	result, err := service.Create(context.Background(), CreateInput{
		CustomerID:          customerID.String(),
		VehicleID:           vehicleID.String(),
		RequestedServiceIDs: []string{serviceID.String()},
		Notes:               "Routine check.",
	})
	require.NoError(t, err)

	assert.Equal(t, StatusReceived, result.Order.Status)
	assert.EqualValues(t, 1, result.Order.Code)
	assert.Len(t, result.Services, 1)
	assert.NotEmpty(t, result.TrackingToken)
}

func TestServiceCreateByDocumentAndPlate(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	seedActiveCustomerAndVehicle(repo)

	result, err := service.Create(context.Background(), CreateInput{
		CustomerDocument: "111.444.777-35",
		LicensePlate:     "ABC1D23",
	})
	require.NoError(t, err)
	assert.Equal(t, StatusReceived, result.Order.Status)
}

func TestServiceCreateCustomerNotFound(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	_, vehicleID := seedActiveCustomerAndVehicle(repo)

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID: uuid.New().String(),
		VehicleID:  vehicleID.String(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCustomerNotFound)
}

func TestServiceCreateCustomerInactive(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID := uuid.New()
	vehicleID := uuid.New()
	repo.addCustomer(&customerRef{ID: customerID, Code: 1, Name: "Carlos", Active: false}, normalizedDocument)
	repo.addVehicle(&vehicleRef{ID: vehicleID, Code: 1, LicensePlate: "ABC1D23", CustomerID: customerID, Active: true})

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID: customerID.String(),
		VehicleID:  vehicleID.String(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCustomerInactive)
}

func TestServiceCreateVehicleNotFound(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, _ := seedActiveCustomerAndVehicle(repo)

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID: customerID.String(),
		VehicleID:  uuid.New().String(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVehicleNotFound)
}

func TestServiceCreateVehicleInactive(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID := uuid.New()
	vehicleID := uuid.New()
	repo.addCustomer(&customerRef{ID: customerID, Code: 1, Name: "Maria", Active: true}, normalizedDocument)
	repo.addVehicle(&vehicleRef{ID: vehicleID, Code: 1, LicensePlate: "ABC1D23", CustomerID: customerID, Active: false})

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID: customerID.String(),
		VehicleID:  vehicleID.String(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVehicleInactive)
}

func TestServiceCreateVehicleNotOwnedByCustomer(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, _ := seedActiveCustomerAndVehicle(repo)

	otherCustomerID := uuid.New()
	repo.addCustomer(&customerRef{ID: otherCustomerID, Code: 2, Name: "Other", Active: true}, "98765432100")
	otherVehicleID := uuid.New()
	repo.addVehicle(&vehicleRef{ID: otherVehicleID, Code: 2, LicensePlate: "XYZ9W88", CustomerID: otherCustomerID, Active: true})

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID: customerID.String(),
		VehicleID:  otherVehicleID.String(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVehicleNotOwnedByCustomer)
}

func TestServiceCreateRequestedServiceNotFound(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID:          customerID.String(),
		VehicleID:           vehicleID.String(),
		RequestedServiceIDs: []string{uuid.New().String()},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestedServiceNotFound)
}

func TestServiceCreateAllowsNoRequestedServices(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	result, err := service.Create(context.Background(), CreateInput{
		CustomerID: customerID.String(),
		VehicleID:  vehicleID.String(),
	})
	require.NoError(t, err)
	assert.Empty(t, result.Services)
}

func TestServiceCreateWithRequestedProducts(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 7, Name: "Brake Pad", Active: true})
	repo.setProductStock(productID, 15)

	result, err := service.Create(context.Background(), CreateInput{
		CustomerID:        customerID.String(),
		VehicleID:         vehicleID.String(),
		RequestedProducts: []RequestedProductInput{{ProductID: productID.String(), Quantity: 2}},
	})
	require.NoError(t, err)

	assert.Equal(t, []RequestedProduct{{ProductID: productID, Quantity: 2}}, result.Order.RequestedProducts)
	require.Len(t, result.Products, 1)
	assert.Equal(t, "Brake Pad", result.Products[0].Product.Name)
	assert.EqualValues(t, 7, result.Products[0].Product.Code)
	assert.Equal(t, 2, result.Products[0].Quantity)
	assert.Equal(t, 15, repo.productStock[productID], "opening an order must not touch stock (requirements.md §7a.1)")
	assert.Empty(t, repo.movements)
}

func TestServiceCreateRequestedProductNotFound(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID:        customerID.String(),
		VehicleID:         vehicleID.String(),
		RequestedProducts: []RequestedProductInput{{ProductID: uuid.New().String(), Quantity: 1}},
	})
	assert.ErrorIs(t, err, ErrRequestedProductNotFound)
	assert.Empty(t, repo.orders, "no order may be persisted")
}

func TestServiceCreateRequestedProductInactive(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Old Part", Active: false})

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID:        customerID.String(),
		VehicleID:         vehicleID.String(),
		RequestedProducts: []RequestedProductInput{{ProductID: productID.String(), Quantity: 1}},
	})
	assert.ErrorIs(t, err, ErrRequestedProductNotFound)
	assert.Empty(t, repo.orders, "no order may be persisted")
}

func TestServiceCreateRequestedProductInvalidQuantity(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true})

	for _, quantity := range []int{0, -3} {
		_, err := service.Create(context.Background(), CreateInput{
			CustomerID:        customerID.String(),
			VehicleID:         vehicleID.String(),
			RequestedProducts: []RequestedProductInput{{ProductID: productID.String(), Quantity: quantity}},
		})
		assert.ErrorIs(t, err, ErrInvalidRequestedProductQuantity, "quantity %d", quantity)
	}
	assert.Empty(t, repo.orders, "no order may be persisted")
}

func TestServiceCreateRejectsDuplicateRequestedProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true})

	_, err := service.Create(context.Background(), CreateInput{
		CustomerID: customerID.String(),
		VehicleID:  vehicleID.String(),
		RequestedProducts: []RequestedProductInput{
			{ProductID: productID.String(), Quantity: 1},
			{ProductID: productID.String(), Quantity: 2},
		},
	})
	assert.ErrorIs(t, err, ErrDuplicateRequestedProduct)
	assert.Empty(t, repo.orders, "no order may be persisted")
}
