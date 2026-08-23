package vehicle

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService() (*VehicleService, *fakeCustomerLookup) {
	customers := newFakeCustomerLookup()
	return NewVehicleService(newFakeRepository(), customers), customers
}

func TestServiceCreate(t *testing.T) {
	service, customers := newTestService()
	customerID := uuid.New()
	customers.addActive(customerID)

	vehicle, err := service.Create(context.Background(), "abc-1d23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	assert.Equal(t, StatusActive, vehicle.Status)
	assert.EqualValues(t, 1, vehicle.Code)
	assert.Equal(t, "ABC1D23", vehicle.LicensePlate)
}

func TestServiceCreateRejectsNonexistentCustomer(t *testing.T) {
	service, _ := newTestService()

	_, err := service.Create(context.Background(), "ABC1D23", "Fiat", "Uno", 2018, "White", uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCustomerNotFound)
}

func TestServiceCreateRejectsInactiveCustomer(t *testing.T) {
	service, customers := newTestService()
	customerID := uuid.New()
	customers.addInactive(customerID)

	_, err := service.Create(context.Background(), "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCustomerInactive)
}

func TestServiceCreateDuplicatePlate(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	_, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	_, err = service.Create(ctx, "abc1d23", "Volkswagen", "Gol", 2020, "Silver", customerID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicatePlate)
}

func TestServiceCreateInvalidPlate(t *testing.T) {
	service, customers := newTestService()
	customerID := uuid.New()
	customers.addActive(customerID)

	_, err := service.Create(context.Background(), "not-a-plate", "Fiat", "Uno", 2018, "White", customerID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPlate)
}

func TestServiceCreateYearOutOfRange(t *testing.T) {
	service, customers := newTestService()
	customerID := uuid.New()
	customers.addActive(customerID)

	_, err := service.Create(context.Background(), "ABC1D23", "Fiat", "Uno", 1900, "White", customerID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidYear)
}

func TestServiceGetNotFound(t *testing.T) {
	service, _ := newTestService()

	_, err := service.Get(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestServiceGetByPlateNormalizesInput(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	created, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	found, err := service.GetByPlate(ctx, " abc-1d23 ")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestServiceList(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	_, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)
	_, err = service.Create(ctx, "DEF4E56", "Volkswagen", "Gol", 2020, "Silver", customerID)
	require.NoError(t, err)
	_, err = service.Create(ctx, "GHI7F89", "Chevrolet", "Onix", 2022, "Black", customerID)
	require.NoError(t, err)

	firstPage, total, err := service.List(ctx, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, firstPage, 2)

	secondPage, total, err := service.List(ctx, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, secondPage, 1)
}

func TestServiceListByCustomer(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerOne, customerTwo := uuid.New(), uuid.New()
	customers.addActive(customerOne)
	customers.addActive(customerTwo)

	_, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerOne)
	require.NoError(t, err)
	_, err = service.Create(ctx, "DEF4E56", "Volkswagen", "Gol", 2020, "Silver", customerTwo)
	require.NoError(t, err)

	vehicles, total, err := service.ListByCustomer(ctx, customerOne, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, vehicles, 1)
	assert.Equal(t, customerOne, vehicles[0].CustomerID)
}

func TestServiceListByCustomerRejectsNonexistentCustomer(t *testing.T) {
	service, _ := newTestService()

	_, _, err := service.ListByCustomer(context.Background(), uuid.New(), 1, 20)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCustomerNotFound)
}

func TestServiceListByCustomerIncludesInactiveCustomersVehicles(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	_, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	// The customer becomes inactive after the vehicle already exists — its
	// vehicles must stay listable (requirements.md BR8 applied to reads).
	customers.addInactive(customerID)

	vehicles, total, err := service.ListByCustomer(ctx, customerID, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, vehicles, 1)
}

func TestServiceUpdatePartial(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	vehicle, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	newColor := "Red"
	updated, err := service.Update(ctx, vehicle.ID, UpdateInput{Color: &newColor})
	require.NoError(t, err)

	assert.Equal(t, "Red", updated.Color)
	// Fields not sent must remain unchanged.
	assert.Equal(t, "Fiat", updated.Brand)
	assert.Equal(t, "Uno", updated.Model)
	assert.Equal(t, 2018, updated.Year)
	// License plate is never part of the update contract.
	assert.Equal(t, "ABC1D23", updated.LicensePlate)
}

func TestServiceUpdateRevalidatesYear(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	vehicle, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	invalidYear := 1900
	_, err = service.Update(ctx, vehicle.ID, UpdateInput{Year: &invalidYear})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidYear)
}

func TestServiceUpdateNotFound(t *testing.T) {
	service, _ := newTestService()

	color := "Red"
	_, err := service.Update(context.Background(), uuid.New(), UpdateInput{Color: &color})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestServiceDeactivate(t *testing.T) {
	service, customers := newTestService()
	ctx := context.Background()
	customerID := uuid.New()
	customers.addActive(customerID)

	vehicle, err := service.Create(ctx, "ABC1D23", "Fiat", "Uno", 2018, "White", customerID)
	require.NoError(t, err)

	deactivated, err := service.Deactivate(ctx, vehicle.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInactive, deactivated.Status)

	// Deactivating again is a no-op, not an error, and the vehicle stays
	// queryable (requirements.md BR7/BR8).
	deactivatedAgain, err := service.Deactivate(ctx, vehicle.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInactive, deactivatedAgain.Status)

	stillFound, err := service.Get(ctx, vehicle.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInactive, stillFound.Status)
}

func TestServiceDeactivateNotFound(t *testing.T) {
	service, _ := newTestService()

	_, err := service.Deactivate(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}
