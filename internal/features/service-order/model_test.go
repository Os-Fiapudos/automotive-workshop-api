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

	order, err := NewServiceOrder(customerID, vehicleID, "Customer reported a noise.", []uuid.UUID{serviceID})
	require.NoError(t, err)

	assert.Equal(t, StatusReceived, order.Status)
	assert.Equal(t, customerID, order.CustomerID)
	assert.Equal(t, vehicleID, order.VehicleID)
	assert.Equal(t, []uuid.UUID{serviceID}, order.RequestedServiceIDs)
}

func TestNewServiceOrderAllowsNoRequestedServices(t *testing.T) {
	order, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil)
	require.NoError(t, err)
	assert.Equal(t, StatusReceived, order.Status)
	assert.Empty(t, order.RequestedServiceIDs)
}

func TestNewServiceOrderRejectsMissingCustomerOrVehicle(t *testing.T) {
	_, err := NewServiceOrder(uuid.Nil, uuid.New(), "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAggregate)

	_, err = NewServiceOrder(uuid.New(), uuid.Nil, "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAggregate)
}
