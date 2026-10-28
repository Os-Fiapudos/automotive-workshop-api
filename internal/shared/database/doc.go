// Package database is the shared PostgreSQL adapter: it builds the
// connection pool (pgx) used by every feature's repository layer (customer,
// auth, ...) and converts money.Cents to and from NUMERIC (money.go), so the
// pgx dependency stays here and out of the pure internal/shared/money type.
package database
