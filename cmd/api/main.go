package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/customer"
	"automotive-workshop-api/internal/features/product"
	servicecatalog "automotive-workshop-api/internal/features/service-catalog"
	serviceorder "automotive-workshop-api/internal/features/service-order"
	servicetracking "automotive-workshop-api/internal/features/service-order-tracking"
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

	quoteNotifier := buildQuoteNotifier(configuration, nil)
	if configuration.ResendAPIKey == "" {
		log.Println("RESEND_API_KEY not set - quote-sent e-mail notifications are disabled")
	}

	serviceOrderRepository := serviceorder.NewPostgresServiceOrderRepository(pool)
	// Shared secret for the external quote-decision channel
	// (specs/service-order-quote-decision/design.md §5.4): fails fast like
	// JWT_SECRET, so a misconfigured deployment is caught at startup rather
	// than on the first external call.
	externalNotificationSecret := os.Getenv("EXTERNAL_NOTIFICATION_SECRET")
	if externalNotificationSecret == "" {
		log.Fatalf("EXTERNAL_NOTIFICATION_SECRET environment variable is not set")
	}
	// Shared secret for the external status-update tool
	// (specs/service-order-status-update/design.md §3): required, and must
	// differ from the quote-decision secret so each integration can be
	// rotated/revoked on its own (requirements.md BR4).
	externalStatusUpdateSecret := os.Getenv("EXTERNAL_STATUS_UPDATE_SECRET")
	if externalStatusUpdateSecret == "" {
		log.Fatalf("EXTERNAL_STATUS_UPDATE_SECRET environment variable is not set")
	}
	if externalStatusUpdateSecret == externalNotificationSecret {
		log.Fatalf("EXTERNAL_STATUS_UPDATE_SECRET must differ from EXTERNAL_NOTIFICATION_SECRET")
	}
	serviceOrderUseCases := buildServiceOrderUseCases(serviceOrderRepository, quoteNotifier, externalNotificationSecret, externalStatusUpdateSecret)

	productRepository := product.NewPostgresProductRepository(pool)
	productService := product.NewProductService(productRepository)

	trackingRepository := servicetracking.NewPostgresTrackingRepository(pool)
	trackingService := servicetracking.NewTrackingService(trackingRepository)

	router := http.NewServeMux()

	// Public routes (auth FR6).
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.Printf("health: encoding response failed: %v", err)
		}
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

	// Customer Management routes now require JWT on every route (RNF02 —
	// specs/auth/design.md §7's "every non-public route requires auth"
	// convention). Resolves the open decision recorded in CLAUDE.md §1/§17.2:
	// these routes were unauthenticated, and one of them
	// (GET /api/v1/customers/document/{document}) let anyone retrieve a
	// customer's name/phone/e-mail from a guessed/generated CPF/CNPJ with no
	// credential at all — see docs/owasp-vulnerability-and-coverage-report.md
	// VULN-01.
	customer.RegisterRoutes(router, customerService, requireAuth)

	// Vehicle Management routes require JWT on every route (RNF02 —
	// specs/vehicle-management/requirements.md §6) — reuses the same
	// requireAuth middleware built for the auth feature.
	vehicle.RegisterRoutes(router, vehicleService, requireAuth)

	// Service Order Opening's POST /api/v1/service-orders route now requires
	// JWT like every other route in this feature. Resolves the open decision
	// recorded in CLAUDE.md §1: the route accepted a customer document/plate
	// as an alternate identifier, so combined with the customer lookup above
	// it let an unauthenticated caller confirm a CPF/CNPJ was a registered
	// customer and open orders in their name — see
	// docs/owasp-vulnerability-and-coverage-report.md VULN-02.
	serviceorder.RegisterRoutes(router, serviceOrderUseCases, requireAuth)

	// Service Order Tracking (specs/service-order-tracking, RF12): a
	// customer-facing route that deliberately never requires the
	// administrative JWT (requirements.md §4/AC10) — it validates its own
	// tracking token instead, so it is registered unwrapped, like Service
	// Order Opening's own creation route above.
	servicetracking.RegisterRoutes(router, trackingService)

	// Explicit timeouts instead of http.ListenAndServe: a server with no read timeout
	// keeps slow or idle connections open indefinitely, which is a denial-of-service
	// vector (gosec G114, CWE-676 — docs/security-report.md, finding SAST-02).
	server := &http.Server{
		Addr:              ":" + configuration.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("listening on :%s", configuration.Port)
	log.Fatal(server.ListenAndServe())
}
