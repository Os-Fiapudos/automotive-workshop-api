package servicetracking

import "errors"

var (
	// ErrOrderNotFound is returned when {codigo} does not identify any
	// service order (requirements.md AC3) — checked before the token, so it
	// maps to 404 regardless of what token was supplied.
	ErrOrderNotFound = errors.New("service order not found")

	// ErrTokenInvalid is returned when the order exists but the supplied
	// token is missing, malformed, doesn't match, belongs to a different
	// order, or has been revoked (requirements.md AC2, AC4, AC5) — all of
	// these are deliberately indistinguishable from the caller's
	// perspective (401), so a wrong token never reveals which specific way
	// it was wrong.
	ErrTokenInvalid = errors.New("tracking token is invalid, revoked, or does not match this service order")
)
