package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"automotive-workshop-api/internal/features/customer"
	"automotive-workshop-api/internal/shared/config"
	"automotive-workshop-api/internal/shared/database"
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

	customerRepository := customer.NewPostgresCustomerRepository(pool)
	customerService := customer.NewCustomerService(customerRepository)

	router := http.NewServeMux()
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	customer.RegisterRoutes(router, customerService)

	log.Printf("listening on :%s", configuration.Port)
	log.Fatal(http.ListenAndServe(":"+configuration.Port, router))
}
