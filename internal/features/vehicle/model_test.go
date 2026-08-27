package vehicle

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVehicleStartsActive(t *testing.T) {
	vehicle, err := NewVehicle("abc-1d23", "Fiat", "Uno", 2018, "White", uuid.New())
	require.NoError(t, err)

	assert.Equal(t, StatusActive, vehicle.Status)
	assert.True(t, vehicle.IsActive())
	assert.Equal(t, "ABC1D23", vehicle.LicensePlate)
}

func TestNewVehicleRejectsInvalidPlate(t *testing.T) {
	_, err := NewVehicle("not-a-plate", "Fiat", "Uno", 2018, "White", uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPlate)
}

func TestNewVehicleRejectsYearOutOfRange(t *testing.T) {
	_, err := NewVehicle("ABC1D23", "Fiat", "Uno", 1949, "White", uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidYear)

	nextYear := time.Now().Year() + 2
	_, err = NewVehicle("ABC1D23", "Fiat", "Uno", nextYear, "White", uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidYear)
}

func TestNewVehicleAcceptsBoundaryYears(t *testing.T) {
	_, err := NewVehicle("ABC1D23", "Fiat", "Uno", minYear, "White", uuid.New())
	require.NoError(t, err)

	_, err = NewVehicle("DEF4E56", "Fiat", "Uno", time.Now().Year()+1, "White", uuid.New())
	require.NoError(t, err)
}

func TestDeactivateIsIdempotent(t *testing.T) {
	vehicle, err := NewVehicle("ABC1D23", "Fiat", "Uno", 2018, "White", uuid.New())
	require.NoError(t, err)

	vehicle.Deactivate()
	assert.Equal(t, StatusInactive, vehicle.Status)
	assert.False(t, vehicle.IsActive())

	// Deactivating an already-inactive vehicle must not error or panic —
	// it's a no-op (see requirements.md BR6/BR7: there is no Activate
	// method, so this call can never revert the status).
	vehicle.Deactivate()
	assert.Equal(t, StatusInactive, vehicle.Status)
}

func TestUpdateDetailsValidatesYear(t *testing.T) {
	vehicle, err := NewVehicle("ABC1D23", "Fiat", "Uno", 2018, "White", uuid.New())
	require.NoError(t, err)

	err = vehicle.UpdateDetails("Fiat", "Uno Mille", 2019, "Red")
	require.NoError(t, err)
	assert.Equal(t, "Uno Mille", vehicle.Model)
	assert.Equal(t, 2019, vehicle.Year)
	assert.Equal(t, "Red", vehicle.Color)
	// License plate is never touched by UpdateDetails.
	assert.Equal(t, "ABC1D23", vehicle.LicensePlate)

	err = vehicle.UpdateDetails("Fiat", "Uno Mille", 1900, "Red")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidYear)
}
