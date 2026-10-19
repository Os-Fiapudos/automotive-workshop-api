package serviceorder

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceOrderStartsReceived(t *testing.T) {
	customerID := uuid.New()
	vehicleID := uuid.New()
	serviceID := uuid.New()

	order, err := NewServiceOrder(customerID, vehicleID, "Customer reported a noise.", []uuid.UUID{serviceID}, nil)
	require.NoError(t, err)

	assert.Equal(t, StatusReceived, order.Status)
	assert.Equal(t, customerID, order.CustomerID)
	assert.Equal(t, vehicleID, order.VehicleID)
	assert.Equal(t, []uuid.UUID{serviceID}, order.RequestedServiceIDs)
}

func TestNewServiceOrderAllowsNoRequestedServices(t *testing.T) {
	order, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, StatusReceived, order.Status)
	assert.Empty(t, order.RequestedServiceIDs)
}

func TestNewServiceOrderRejectsMissingCustomerOrVehicle(t *testing.T) {
	_, err := NewServiceOrder(uuid.Nil, uuid.New(), "", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAggregate)

	_, err = NewServiceOrder(uuid.New(), uuid.Nil, "", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAggregate)
}

func TestNewServiceOrderKeepsRequestedProducts(t *testing.T) {
	requested := []RequestedProduct{{ProductID: uuid.New(), Quantity: 2}, {ProductID: uuid.New(), Quantity: 1}}

	order, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil, requested)
	require.NoError(t, err)
	assert.Equal(t, StatusReceived, order.Status)
	assert.Equal(t, requested, order.RequestedProducts)
}

func TestNewServiceOrderRejectsNonPositiveProductQuantity(t *testing.T) {
	for _, quantity := range []int{0, -1} {
		_, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil, []RequestedProduct{{ProductID: uuid.New(), Quantity: quantity}})
		assert.ErrorIs(t, err, ErrInvalidRequestedProductQuantity, "quantity %d", quantity)
	}
}

func TestNewServiceOrderRejectsDuplicateRequestedProduct(t *testing.T) {
	productID := uuid.New()

	_, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil, []RequestedProduct{
		{ProductID: productID, Quantity: 1},
		{ProductID: productID, Quantity: 3},
	})
	assert.ErrorIs(t, err, ErrDuplicateRequestedProduct)
}
