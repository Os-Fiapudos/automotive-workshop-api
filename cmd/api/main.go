package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/customer"
	"automotive-workshop-api/internal/features/product"
	serviceorder "automotive-workshop-api/internal/features/service-order"
	"automotive-workshop-api/internal/features/servicecatalog"
	"automotive-workshop-api/internal/shared/config"
	"automotive-workshop-api/internal/shared/database"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

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

	// Service Order Opening routes remain unauthenticated for now, same
	// rationale as Customer Management above.
	serviceorder.RegisterRoutes(router, serviceOrderService)

	log.Printf("listening on :%s", configuration.Port)
	log.Fatal(http.ListenAndServe(":"+configuration.Port, router))
}
