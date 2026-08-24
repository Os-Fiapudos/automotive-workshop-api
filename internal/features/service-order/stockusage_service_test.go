package serviceorder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterStockUsageSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: true})
	repo.setProductStock(productID, 10)

	movements, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
		{ProductID: productID.String(), Quantity: 3},
	})
	require.NoError(t, err)
	require.Len(t, movements, 1)
	assert.Equal(t, StockMovementExit, movements[0].Type)
	assert.Equal(t, 10, movements[0].PreviousStock)
	assert.Equal(t, 7, movements[0].NewStock)
	assert.Equal(t, order.ID, *movements[0].ServiceOrderID)
}

// TestRegisterStockUsageRejectsWithoutEmExecucao covers BR1: a deduction can
// only be registered while the order is EM_EXECUCAO.
func TestRegisterStockUsageRejectsWithoutEmExecucao(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: true})
	repo.setProductStock(productID, 10)

	for _, status := range []Status{StatusRecebida, StatusEmDiagnostico, StatusAguardandoAprovacao, StatusFinalizada, StatusEntregue} {
		order := seedOrder(repo, status)
		_, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
			{ProductID: productID.String(), Quantity: 1},
		})
		assert.ErrorIs(t, err, ErrInvalidStatusTransition, "status %s must be rejected", status)
	}
}

// TestRegisterStockUsageRejectsInvalidQuantity covers BR3.
func TestRegisterStockUsageRejectsInvalidQuantity(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: true})
	repo.setProductStock(productID, 10)

	_, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
		{ProductID: productID.String(), Quantity: 0},
	})
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestRegisterStockUsageRejectsEmptyItems(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)

	_, err := service.RegisterStockUsage(context.Background(), order.ID, nil)
	assert.ErrorIs(t, err, ErrEmptyStockUsage)
}

// TestRegisterStockUsageRejectsInsufficientStock covers BR4 via the fake
// repository's atomic-update replica (design.md §9).
func TestRegisterStockUsageRejectsInsufficientStock(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: true})
	repo.setProductStock(productID, 2)

	_, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
		{ProductID: productID.String(), Quantity: 5},
	})
	assert.ErrorIs(t, err, ErrInsufficientStock)
}

func TestRegisterStockUsageRejectsInactiveProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: false})
	repo.setProductStock(productID, 10)

	_, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
		{ProductID: productID.String(), Quantity: 1},
	})
	assert.ErrorIs(t, err, ErrProductInactive)
}

func TestReverseStockMovementSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: true})
	repo.setProductStock(productID, 10)

	movements, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
		{ProductID: productID.String(), Quantity: 4},
	})
	require.NoError(t, err)

	reversal, err := service.ReverseStockMovement(context.Background(), order.ID, movements[0].ID)
	require.NoError(t, err)
	assert.Equal(t, StockMovementEntry, reversal.Type)
	assert.Equal(t, movements[0].ID, *reversal.ReversedMovementID)
	assert.Equal(t, 10, reversal.NewStock)

	_, err = service.ReverseStockMovement(context.Background(), order.ID, movements[0].ID)
	assert.ErrorIs(t, err, ErrStockMovementAlreadyReversed)
}

func TestListStockMovements(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmExecucao)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Active: true})
	repo.setProductStock(productID, 10)

	_, err := service.RegisterStockUsage(context.Background(), order.ID, []StockUsageItem{
		{ProductID: productID.String(), Quantity: 4},
	})
	require.NoError(t, err)

	movements, err := service.ListStockMovements(context.Background(), order.ID)
	require.NoError(t, err)
	require.Len(t, movements, 1)
	assert.Equal(t, 4, movements[0].Quantity)
}
