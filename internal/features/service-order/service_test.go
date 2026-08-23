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
	return NewServiceOrderService(repo, repo)
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

	assert.Equal(t, StatusRecebida, result.Order.Status)
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
	assert.Equal(t, StatusRecebida, result.Order.Status)
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
