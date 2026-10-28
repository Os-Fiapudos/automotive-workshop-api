// Package money is the exact monetary value type shared by every feature that
// holds a price or a total (service-catalog, product, service-order). Its one
// rule: no float64 ever touches a Cents value, in either direction — JSON is
// parsed and formatted from decimal text, and arithmetic is int64 arithmetic
// bounded by the NUMERIC(12,2) columns the values are stored in.
//
// The package depends only on the standard library on purpose: the conversion
// to and from Postgres' NUMERIC lives in internal/shared/database, the
// persistence adapter, so domain code importing money does not transitively
// depend on pgx (specs/monetary-value-precision/design.md §1).
package money
