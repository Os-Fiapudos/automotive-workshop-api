package domain

import (
	"time"

	"github.com/google/uuid"
)

// ServiceOrderHistory is a single audit-trail entry of a ServiceOrder's
// status changes (docs/entities.md), read-only from the query use cases'
// perspective — every row is written by service-order-opening (the
// "creation" event) or by a status transition ("diagnosis_started",
// "quote_composed", ...); specs/service-order-query/ only ever reads them back.
type ServiceOrderHistory struct {
	ID             uuid.UUID
	ServiceOrderID uuid.UUID
	OccurredAt     time.Time
	Event          string
	Description    string
	PreviousStatus Status
	NewStatus      Status
}

// Fixed history descriptions of the transitions the external status update
// can reach — the same text the administrative endpoints have always
// written (specs/service-order-status-update/requirements.md AC10).
const (
	HistoryDiagnosisStarted = "Diagnosis started."
	HistoryOrderFinalized   = "Service order finalized."
	HistoryVehicleDelivered = "Vehicle delivered to customer."
)

// HistoryDescription appends notes, when present, to a transition's fixed
// history description (specs/service-order-status-update/design.md §2), so
// an external update's notes are persisted with the event itself instead of
// being dropped.
func HistoryDescription(base, notes string) string {
	if notes == "" {
		return base
	}
	return base + " Notes: " + notes
}
