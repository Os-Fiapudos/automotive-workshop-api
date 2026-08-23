package customer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCustomerStartsActive(t *testing.T) {
	customer, err := NewCustomer("Maria Silva", "111.444.777-35", "+55 11 91234-5678", nil)
	require.NoError(t, err)

	assert.Equal(t, StatusActive, customer.Status)
	assert.True(t, customer.IsActive())
	assert.Equal(t, "11144477735", customer.Document.Value)
}

func TestNewCustomerRejectsInvalidDocument(t *testing.T) {
	_, err := NewCustomer("Maria Silva", "000.000.000-00", "+55 11 91234-5678", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDocument)
}

func TestDeactivateIsIdempotent(t *testing.T) {
	customer, err := NewCustomer("Maria Silva", "111.444.777-35", "+55 11 91234-5678", nil)
	require.NoError(t, err)

	customer.Deactivate()
	assert.Equal(t, StatusInactive, customer.Status)
	assert.False(t, customer.IsActive())

	// Deactivating an already-inactive customer must not error or panic —
	// it's a no-op (see requirements.md §3.6/§3.7: there is no Activate
	// method, so this call can never revert the status).
	customer.Deactivate()
	assert.Equal(t, StatusInactive, customer.Status)
}

func TestChangeDocumentValidatesAndNormalizes(t *testing.T) {
	customer, err := NewCustomer("Maria Silva", "111.444.777-35", "+55 11 91234-5678", nil)
	require.NoError(t, err)

	err = customer.ChangeDocument("11.222.333/0001-81")
	require.NoError(t, err)
	assert.Equal(t, "11222333000181", customer.Document.Value)

	err = customer.ChangeDocument("not-a-document")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDocument)
}
