package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/shared/database"
	"automotive-workshop-api/internal/shared/httpx"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

func main() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	ttl := time.Hour
	if raw := os.Getenv("JWT_TTL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			log.Fatalf("invalid JWT_TTL %q: %v", raw, err)
		}
		ttl = parsed
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := database.Connect(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()

	tokens := token.NewManager(secret, ttl)
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), tokens))
	requireAuth := middleware.RequireAuth(tokens)

	mux := http.NewServeMux()

	// Public routes (FR6). Everything not listed here must be registered
	// wrapped in requireAuth.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)

	// Protected routes.
	mux.Handle("GET /api/v1/auth/me", requireAuth(http.HandlerFunc(authHandler.Me)))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
