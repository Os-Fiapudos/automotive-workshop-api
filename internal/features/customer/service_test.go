package customer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validCPF1  = "111.444.777-35"
	validCPF2  = "529.982.247-25"
	validCNPJ1 = "11.222.333/0001-81"
)

func newTestService() *CustomerService {
	return NewCustomerService(newFakeRepository())
}

func TestServiceCreate(t *testing.T) {
	service := newTestService()

	customer, err := service.Create(context.Background(), "Maria Silva", validCPF1, "+55 11 91234-5678", nil)
	require.NoError(t, err)

	assert.Equal(t, StatusActive, customer.Status)
	assert.EqualValues(t, 1, customer.Code)
	assert.Equal(t, "11144477735", customer.Document.Value)
}

func TestServiceCreateDuplicateDocument(t *testing.T) {
	service := newTestService()
	ctx := context.Background()

	_, err := service.Create(ctx, "Maria Silva", validCPF1, "+55 11 91234-5678", nil)
	require.NoError(t, err)

	_, err = service.Create(ctx, "Someone Else", validCPF1, "+55 11 90000-0000", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateDocument)
}

func TestServiceCreateInvalidDocument(t *testing.T) {
	service := newTestService()

	_, err := service.Create(context.Background(), "Maria Silva", "000.000.000-00", "+55 11 91234-5678", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDocument)
}

func TestServiceGetNotFound(t *testing.T) {
	service := newTestService()

	_, err := service.Get(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestServiceGetByDocumentNormalizesInput(t *testing.T) {
	service := newTestService()
	ctx := context.Background()

	created, err := service.Create(ctx, "Maria Silva", validCPF1, "+55 11 91234-5678", nil)
	require.NoError(t, err)

	found, err := service.GetByDocument(ctx, "  111.444.777-35 ")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestServiceList(t *testing.T) {
	service := newTestService()
	ctx := context.Background()

	_, err := service.Create(ctx, "Customer 1", validCPF1, "+55 11 90000-0001", nil)
	require.NoError(t, err)
	_, err = service.Create(ctx, "Customer 2", validCPF2, "+55 11 90000-0002", nil)
	require.NoError(t, err)
	_, err = service.Create(ctx, "Customer 3", validCNPJ1, "+55 11 90000-0003", nil)
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

func TestServiceUpdatePartial(t *testing.T) {
	service := newTestService()
	ctx := context.Background()

	customer, err := service.Create(ctx, "Maria Silva", validCPF1, "+55 11 91234-5678", nil)
	require.NoError(t, err)

	newName := "Maria Silva Santos"
	updated, err := service.Update(ctx, customer.ID, UpdateInput{Name: &newName})
	require.NoError(t, err)

	assert.Equal(t, "Maria Silva Santos", updated.Name)
	// Fields not sent must remain unchanged.
	assert.Equal(t, "+55 11 91234-5678", updated.Phone)
	assert.Equal(t, "11144477735", updated.Document.Value)
}

func TestServiceUpdateDocumentRevalidatesAndChecksUniqueness(t *testing.T) {
	service := newTestService()
	ctx := context.Background()

	_, err := service.Create(ctx, "Customer 1", validCPF1, "+55 11 90000-0001", nil)
	require.NoError(t, err)
	secondCustomer, err := service.Create(ctx, "Customer 2", validCPF2, "+55 11 90000-0002", nil)
	require.NoError(t, err)

	// Updating customer 2's document to customer 1's document must fail.
	duplicateDocument := validCPF1
	_, err = service.Update(ctx, secondCustomer.ID, UpdateInput{Document: &duplicateDocument})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateDocument)

	// An invalid document must fail validation before any uniqueness check.
	invalidDocument := "not-a-document"
	_, err = service.Update(ctx, secondCustomer.ID, UpdateInput{Document: &invalidDocument})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDocument)

	// A fresh, valid, unused document must succeed.
	newDocument := validCNPJ1
	updated, err := service.Update(ctx, secondCustomer.ID, UpdateInput{Document: &newDocument})
	require.NoError(t, err)
	assert.Equal(t, "11222333000181", updated.Document.Value)
}

func TestServiceUpdateNotFound(t *testing.T) {
	service := newTestService()

	name := "Someone"
	_, err := service.Update(context.Background(), uuid.New(), UpdateInput{Name: &name})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestServiceDeactivate(t *testing.T) {
	service := newTestService()
	ctx := context.Background()

	customer, err := service.Create(ctx, "Maria Silva", validCPF1, "+55 11 91234-5678", nil)
	require.NoError(t, err)

	deactivated, err := service.Deactivate(ctx, customer.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInactive, deactivated.Status)

	// Deactivating again is a no-op, not an error, and the customer stays
	// queryable (see requirements.md §3.6/§3.8).
	deactivatedAgain, err := service.Deactivate(ctx, customer.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInactive, deactivatedAgain.Status)

	stillFound, err := service.Get(ctx, customer.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInactive, stillFound.Status)
}

func TestServiceDeactivateNotFound(t *testing.T) {
	service := newTestService()

	_, err := service.Deactivate(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}
