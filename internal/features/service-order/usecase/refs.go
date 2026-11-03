package usecase

import (
	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/money"
)

// The read models below are this feature's own projections of other
// features' tables (customers, vehicles, services, products), populated by
// raw SQL in the Postgres adapter — deliberately not the customer, vehicle,
// service-catalog or product features' Go types: importing those packages
// would violate the "no direct coupling between features" rule
// (CLAUDE.md §9.2, specs/service-order-architecture-refactor/design.md §4).

// CustomerRef is the minimal customer data this feature needs to resolve
// and validate a customer, plus display it in the response.
type CustomerRef struct {
	ID       uuid.UUID
	Code     int64
	Name     string
	Active   bool
	Document string
	Phone    string
	// Email is nullable (customers.email); only the quote-sent notification
	// reads it (specs/service-order-email-notification/).
	Email *string
}

// VehicleRef is the vehicle-side equivalent of CustomerRef. Brand/Model/Year/
// Color are only needed by the detail view (specs/service-order-query/), left
// unused (but always populated — same query as everyone else) by the create
// flow.
type VehicleRef struct {
	ID           uuid.UUID
	Code         int64
	LicensePlate string
	CustomerID   uuid.UUID
	Active       bool
	Brand        string
	Model        string
	Year         int
	Color        string
}

// ServiceRef is the minimal service-catalog data needed to display a
// requested service in the response. Description/Price are only populated
// by FindServiceByID (used to price/snapshot a quote item,
// specs/service-order-diagnosis-quote/) — FindServicesByIDs (used for the
// order-creation response) leaves them zero, since that response doesn't
// need them.
type ServiceRef struct {
	ID          uuid.UUID
	Code        int64
	Name        string
	Description string
	Price       money.Cents
}

// ProductRef is the minimal product-catalog data needed to price and
// describe a quote item, plus its Active flag for the RF-required
// active-product check (specs/service-order-opening/design.md §1.4).
type ProductRef struct {
	ID          uuid.UUID
	Code        int64
	Name        string
	Description string
	UnitPrice   money.Cents
	Active      bool
}
