package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/product"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

func testProductServer(t *testing.T) (*httptest.Server, *pgxpool.Pool, string) {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("skipping integration test: cannot create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: database unreachable: %v", err)
	}
	t.Cleanup(pool.Close)

	jwtSecret := "integration-test-secret"
	tokens := token.NewManager(jwtSecret, time.Hour)
	requireAuth := middleware.RequireAuth(tokens)

	productRepository := product.NewPostgresProductRepository(pool)
	productService := product.NewProductService(productRepository)

	authRepository := auth.NewRepository(pool)
	authService := auth.NewService(authRepository, tokens)
	authHandler := auth.NewHandler(authService)

	router := http.NewServeMux()
	router.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	product.RegisterRoutes(router, productService, requireAuth)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// Obtain valid JWT token for authenticated calls
	loginResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"email":    "admin@workshop.local",
		"password": "admin123",
	}, "")
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	var authBody struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, loginResp, &authBody)
	require.NotEmpty(t, authBody.AccessToken)

	return server, pool, authBody.AccessToken
}

func cleanupProduct(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})
}

func doAuthJSON(t *testing.T, method, url string, body any, authToken string) *http.Response {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	request, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		request.Header.Set("Authorization", "Bearer "+authToken)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

func TestProductUnauthenticatedReturns401(t *testing.T) {
	server, _, _ := testProductServer(t)

	// Attempt POST without token
	resp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Name:      "Filtro Sem Auth",
		UnitPrice: ptr(10.0),
		Type:      "PART",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Attempt GET list without token
	getResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/produtos", nil, "")
	assert.Equal(t, http.StatusUnauthorized, getResp.StatusCode)
}

func TestProductFullCRUDFlow(t *testing.T) {
	server, pool, token := testProductServer(t)

	// 1. Create PART product
	partResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Name:         "Amortecedor Dianteiro",
		Description:  "Amortecedor pressurizado",
		UnitPrice:    ptr(250.00),
		CurrentStock: ptr(12),
		Type:         "PART",
	}, token)
	require.Equal(t, http.StatusCreated, partResp.StatusCode)
	var createdPart product.Response
	decodeBody(t, partResp, &createdPart)
	cleanupProduct(t, pool, createdPart.ID)

	assert.Equal(t, "Amortecedor Dianteiro", createdPart.Name)
	assert.Equal(t, "PART", createdPart.Type)
	assert.Equal(t, "ACTIVE", createdPart.Status)
	assert.Equal(t, 250.00, createdPart.UnitPrice)
	assert.Equal(t, 12, createdPart.CurrentStock)
	assert.NotZero(t, createdPart.Code)

	// 2. Create SUPPLY product
	supplyResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Name:         "Graxa de Lítio",
		Description:  "Pote 1kg",
		UnitPrice:    ptr(35.50),
		CurrentStock: ptr(8),
		Type:         "SUPPLY",
	}, token)
	require.Equal(t, http.StatusCreated, supplyResp.StatusCode)
	var createdSupply product.Response
	decodeBody(t, supplyResp, &createdSupply)
	cleanupProduct(t, pool, createdSupply.ID)

	assert.Equal(t, "SUPPLY", createdSupply.Type)

	// 3. Get product by ID
	getResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/produtos/"+createdPart.ID, nil, token)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched product.Response
	decodeBody(t, getResp, &fetched)
	assert.Equal(t, createdPart.ID, fetched.ID)

	// 4. Update details (name, price) and attempt direct stock update rejection (RNF07)
	rejectStockPatch := doAuthJSON(t, http.MethodPatch, server.URL+"/api/v1/produtos/"+createdPart.ID, product.UpdateRequest{
		CurrentStock: ptr(99),
	}, token)
	assert.Equal(t, http.StatusBadRequest, rejectStockPatch.StatusCode)

	patchResp := doAuthJSON(t, http.MethodPatch, server.URL+"/api/v1/produtos/"+createdPart.ID, product.UpdateRequest{
		Name:      ptr("Amortecedor Dianteiro Turbo"),
		UnitPrice: ptr(275.00),
	}, token)
	require.Equal(t, http.StatusOK, patchResp.StatusCode)
	var updated product.Response
	decodeBody(t, patchResp, &updated)
	assert.Equal(t, "Amortecedor Dianteiro Turbo", updated.Name)
	assert.Equal(t, 275.00, updated.UnitPrice)
	assert.Equal(t, 12, updated.CurrentStock) // stock unchanged

	// 5. List products
	listResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/produtos?page=1&pageSize=100", nil, token)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list product.ListResponse
	decodeBody(t, listResp, &list)
	assert.GreaterOrEqual(t, list.Total, 2)

	// 6. Deactivate (logical delete)
	deactivateResp := doAuthJSON(t, http.MethodDelete, server.URL+"/api/v1/produtos/"+createdPart.ID, nil, token)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)
	var deactivated product.Response
	decodeBody(t, deactivateResp, &deactivated)
	assert.Equal(t, "INACTIVE", deactivated.Status)

	// Verify row physically exists in DB
	var exists bool
	err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM products WHERE id = $1)`, createdPart.ID).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStockAdjustmentFlow(t *testing.T) {
	server, pool, token := testProductServer(t)

	// 1. Create product with stock = 10
	createResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Name:         "Óleo de Câmbio",
		Description:  "Fluido sintético",
		UnitPrice:    ptr(55.00),
		CurrentStock: ptr(10),
		Type:         "SUPPLY",
	}, token)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created product.Response
	decodeBody(t, createResp, &created)
	cleanupProduct(t, pool, created.ID)

	// 2. Adjust entry (+5)
	entryResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos/"+created.ID+"/estoque/ajustes", product.StockAdjustmentRequest{
		Type:     "ENTRY",
		Quantity: 5,
		Reason:   "Ajuste de estoque inicial",
	}, token)
	require.Equal(t, http.StatusCreated, entryResp.StatusCode)
	var entryMove product.StockMovementResponse
	decodeBody(t, entryResp, &entryMove)
	assert.Equal(t, 10, entryMove.PreviousStock)
	assert.Equal(t, 15, entryMove.NewStock)

	// 3. Adjust exit (-4)
	exitResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos/"+created.ID+"/estoque/ajustes", product.StockAdjustmentRequest{
		Type:     "EXIT",
		Quantity: 4,
		Reason:   "Uso em serviço",
	}, token)
	require.Equal(t, http.StatusCreated, exitResp.StatusCode)
	var exitMove product.StockMovementResponse
	decodeBody(t, exitResp, &exitMove)
	assert.Equal(t, 15, exitMove.PreviousStock)
	assert.Equal(t, 11, exitMove.NewStock)

	// 4. Query current stock balance
	balanceResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/produtos/"+created.ID+"/estoque", nil, token)
	require.Equal(t, http.StatusOK, balanceResp.StatusCode)
	var balance product.StockBalanceResponse
	decodeBody(t, balanceResp, &balance)
	assert.Equal(t, 11, balance.CurrentStock)

	// 5. Query movements list
	movementsResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/produtos/"+created.ID+"/movimentacoes", nil, token)
	require.Equal(t, http.StatusOK, movementsResp.StatusCode)

	// 6. Reject excessive exit (insufficient stock) -> 409 Conflict
	excessResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos/"+created.ID+"/estoque/ajustes", product.StockAdjustmentRequest{
		Type:     "EXIT",
		Quantity: 100,
		Reason:   "Saída excessiva",
	}, token)
	assert.Equal(t, http.StatusConflict, excessResp.StatusCode)
	var errBody map[string]any
	decodeBody(t, excessResp, &errBody)
	errObj := errBody["error"].(map[string]any)
	assert.Equal(t, "INSUFFICIENT_STOCK", errObj["code"])

	// 7. Reject zero quantity -> 400 Bad Request
	zeroResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos/"+created.ID+"/estoque/ajustes", product.StockAdjustmentRequest{
		Type:     "ENTRY",
		Quantity: 0,
		Reason:   "Zero quantidade",
	}, token)
	assert.Equal(t, http.StatusBadRequest, zeroResp.StatusCode)
}

func TestStockAdjustmentConcurrency(t *testing.T) {
	server, pool, token := testProductServer(t)

	// Create product with stock = 10
	createResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Name:         "Produto Concorrente",
		Description:  "Teste de concorrência de estoque",
		UnitPrice:    ptr(100.00),
		CurrentStock: ptr(10),
		Type:         "PART",
	}, token)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created product.Response
	decodeBody(t, createResp, &created)
	cleanupProduct(t, pool, created.ID)

	// Launch 10 concurrent exit requests for 2 units each (Total requested: 20 units; Stock: 10 units)
	// Exactly 5 should succeed (5 * 2 = 10) and 5 should fail with 409 Conflict.
	concurrentCount := 10
	var wg sync.WaitGroup
	statusChan := make(chan int, concurrentCount)

	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos/"+created.ID+"/estoque/ajustes", product.StockAdjustmentRequest{
				Type:     "EXIT",
				Quantity: 2,
				Reason:   "Saída concorrente",
			}, token)
			statusChan <- resp.StatusCode
		}()
	}

	wg.Wait()
	close(statusChan)

	successCount := 0
	conflictCount := 0
	for status := range statusChan {
		if status == http.StatusCreated {
			successCount++
		} else if status == http.StatusConflict {
			conflictCount++
		}
	}

	assert.Equal(t, 5, successCount, "Exactly 5 exit operations should succeed")
	assert.Equal(t, 5, conflictCount, "Exactly 5 exit operations should fail with 409 Conflict")

	// Verify final balance is exactly 0
	balanceResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/produtos/"+created.ID+"/estoque", nil, token)
	require.Equal(t, http.StatusOK, balanceResp.StatusCode)
	var balance product.StockBalanceResponse
	decodeBody(t, balanceResp, &balance)
	assert.Equal(t, 0, balance.CurrentStock)
}

func TestProductDuplicateCodeReturns409(t *testing.T) {
	server, pool, token := testProductServer(t)

	customCode := int64(800000 + rand.Intn(100000))

	firstResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Code:         &customCode,
		Name:         "Produto Código Fixo 1",
		Description:  "Desc 1",
		UnitPrice:    ptr(10.0),
		CurrentStock: ptr(5),
		Type:         "PART",
	}, token)
	require.Equal(t, http.StatusCreated, firstResp.StatusCode)
	var p1 product.Response
	decodeBody(t, firstResp, &p1)
	cleanupProduct(t, pool, p1.ID)

	secondResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
		Code:         &customCode,
		Name:         "Produto Código Fixo 2",
		Description:  "Desc 2",
		UnitPrice:    ptr(20.0),
		CurrentStock: ptr(10),
		Type:         "SUPPLY",
	}, token)
	assert.Equal(t, http.StatusConflict, secondResp.StatusCode)

	var errBody map[string]any
	decodeBody(t, secondResp, &errBody)
	errObj := errBody["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_CODE", errObj["code"])
}

func TestProductRejectsInvalidInputs(t *testing.T) {
	server, _, token := testProductServer(t)

	t.Run("negative unit price", func(t *testing.T) {
		resp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
			Name:         "Preço Negativo",
			UnitPrice:    ptr(-10.0),
			CurrentStock: ptr(5),
			Type:         "PART",
		}, token)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("negative stock", func(t *testing.T) {
		resp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
			Name:         "Estoque Negativo",
			UnitPrice:    ptr(10.0),
			CurrentStock: ptr(-5),
			Type:         "PART",
		}, token)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("invalid type", func(t *testing.T) {
		resp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/produtos", product.CreateRequest{
			Name:         "Tipo Inválido",
			UnitPrice:    ptr(10.0),
			CurrentStock: ptr(5),
			Type:         "SERVICO",
		}, token)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func ptr[T any](v T) *T {
	return &v
}

func stringValue(i int) string {
	return strconv.Itoa(i)
}
