package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/customer"
	"automotive-workshop-api/internal/features/product"
	serviceorder "automotive-workshop-api/internal/features/service-order"
	"automotive-workshop-api/internal/features/servicecatalog"
	"automotive-workshop-api/internal/features/vehicle"
	"automotive-workshop-api/internal/shared/config"
	"automotive-workshop-api/internal/shared/database"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

// customerLookupAdapter adapts the already-built *customer.CustomerService to
// vehicle.CustomerLookup, so internal/features/vehicle can check a
// referenced customer's existence/status (requirements.md BR1) without
// importing internal/features/customer directly (CLAUDE.md §9) — mirrors
// specs/vehicle-management/design.md §1.3.
type customerLookupAdapter struct{ service *customer.CustomerService }

func (adapter customerLookupAdapter) IsActiveCustomer(ctx context.Context, id uuid.UUID) (bool, bool, error) {
	found, err := adapter.service.Get(ctx, id)
	if errors.Is(err, customer.ErrNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, found.IsActive(), nil
}

func main() {
	configuration, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, configuration.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	tokens := token.NewManager(configuration.JWTSecret, configuration.JWTTTL)
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), tokens))
	catalogHandler := servicecatalog.NewHandler(servicecatalog.NewCatalog(servicecatalog.NewRepository(pool)))
	requireAuth := middleware.RequireAuth(tokens)

	customerRepository := customer.NewPostgresCustomerRepository(pool)
	customerService := customer.NewCustomerService(customerRepository)

	vehicleRepository := vehicle.NewPostgresVehicleRepository(pool)
	vehicleService := vehicle.NewVehicleService(vehicleRepository, customerLookupAdapter{service: customerService})

	serviceOrderRepository := serviceorder.NewPostgresServiceOrderRepository(pool)
	serviceOrderService := serviceorder.NewServiceOrderService(serviceOrderRepository, serviceOrderRepository)

	productRepository := product.NewPostgresProductRepository(pool)
	productService := product.NewProductService(productRepository)

	router := http.NewServeMux()

	// Public routes (auth FR6).
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	router.HandleFunc("POST /api/v1/auth/login", authHandler.Login)

	// Protected routes.
	router.Handle("GET /api/v1/auth/me", requireAuth(http.HandlerFunc(authHandler.Me)))

	// Service catalog routes (specs/service-catalog, protected per RNF02).
	router.Handle("POST /api/v1/services", requireAuth(http.HandlerFunc(catalogHandler.Create)))
	router.Handle("GET /api/v1/services", requireAuth(http.HandlerFunc(catalogHandler.List)))
	router.Handle("GET /api/v1/services/{id}", requireAuth(http.HandlerFunc(catalogHandler.Get)))
	router.Handle("PATCH /api/v1/services/{id}", requireAuth(http.HandlerFunc(catalogHandler.Update)))
	router.Handle("DELETE /api/v1/services/{id}", requireAuth(http.HandlerFunc(catalogHandler.Delete)))

	// Product management routes (protected via requireAuth per RNF02).
	product.RegisterRoutes(router, productService, requireAuth)

	// Customer Management routes remain unauthenticated for now, matching
	// specs/customer-management/requirements.md §7.2 ("implemented
	// unauthenticated... a dedicated Security feature... will add JWT
	// authentication/authorization as a cross-cutting concern applied on
	// top of the existing routes"). Wrapping them in requireAuth is a
	// one-line follow-up (router.Handle(pattern, requireAuth(handler)) per
	// route, or a small helper) once that's explicitly decided — not done
	// silently as part of this merge.
	customer.RegisterRoutes(router, customerService)

	// Vehicle Management routes require JWT on every route (RNF02 —
	// specs/vehicle-management/requirements.md §6), unlike Customer
	// Management's still-unauthenticated routes above — reuses the same
	// requireAuth middleware built for the auth feature.
	vehicle.RegisterRoutes(router, vehicleService, requireAuth)

	// Service Order Opening's own POST /api/v1/service-orders route remains
	// unauthenticated for now, same rationale as Customer Management above.
	// The diagnosis/quote routes added by
	// specs/service-order-diagnosis-quote/ are protected with requireAuth
	// (requirements.md §7.4) — a deliberate, explicit decision for those
	// routes specifically, not a silent resolution of the open decision
	// above.
	serviceorder.RegisterRoutes(router, serviceOrderService, requireAuth)

	log.Printf("listening on :%s", configuration.Port)
	log.Fatal(http.ListenAndServe(":"+configuration.Port, router))
}
