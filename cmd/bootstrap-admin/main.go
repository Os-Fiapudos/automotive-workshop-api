package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/shared/config"
	"automotive-workshop-api/internal/shared/database"
)

func main() {
	log.SetPrefix("bootstrap-admin: ")
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run keeps main thin (CLAUDE.md §9.4) and lets the deferred pool Close run
// before the process exits. Nothing it logs contains the password or its hash.
func run() error {
	cfg, err := config.LoadAdminBootstrap()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer pool.Close()

	result, err := auth.BootstrapAdmin(ctx, auth.NewRepository(pool), auth.AdminBootstrapInput{
		Name:     cfg.AdminName,
		Email:    cfg.AdminEmail,
		Password: cfg.AdminPassword,
	})
	if errors.Is(err, auth.ErrInvalidAdminEmail) || errors.Is(err, auth.ErrInvalidAdminPassword) {
		return err
	}
	if err != nil {
		return fmt.Errorf("%w (has the schema been migrated? scripts/migrate.sh up)", err)
	}

	if result.Created {
		log.Printf("administrative user created (code %d)", result.Code)
	} else {
		log.Printf("an administrative user with this e-mail already exists - left unchanged (password not modified)")
	}
	return nil
}
