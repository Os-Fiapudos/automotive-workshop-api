package servicetracking

import "time"

// trackingRead is the reduced read projection this feature returns — the
// "projeção de leitura reduzida" from the source spec
// (specs/service-order-tracking/design.md §4). It is never assembled from,
// and never shares a struct with, the administrative
// internal/features/service-order.Response.
type trackingRead struct {
	Code       int64
	Status     string
	Vehicle    trackingVehicle
	Milestones []trackingMilestone
}

// trackingVehicle is a limited, non-identifying-of-the-owner view of the
// vehicle: physical description only, no id/code/status/customer link
// (requirements.md §3.4, AC6).
type trackingVehicle struct {
	LicensePlate string
	Brand        string
	Model        string
	Year         int
	Color        string
}

// trackingMilestone mirrors one ServiceOrderHistory row, minus its free-text
// Description field (requirements.md §0 item 6, AC6).
type trackingMilestone struct {
	Event          string
	PreviousStatus string
	NewStatus      string
	OccurredAt     time.Time
}
