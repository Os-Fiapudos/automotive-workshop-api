// Package postgres is the service-order feature's persistence adapter: one
// Repository, backed by a pgx pool, that implements every persistence port
// declared in usecase/ports.go (assertions.go). All SQL, transactions and
// pgx-to-domain conversions (including pgx.ErrNoRows to the domain's
// not-found errors) live here.
//
// Dependencies point inward: this package imports domain and usecase, never
// the reverse. Reads of other features' tables (customers, vehicles,
// services, products) are raw SQL into usecase read models, not imports of
// those features' packages (CLAUDE.md §9.2).
package postgres
