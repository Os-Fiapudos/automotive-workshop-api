package servicetracking

import "time"

// trackingVehicleDTO/trackingMilestoneDTO/trackingResponse are the public
// contract for GET /api/v1/acompanhamento/{codigo} — a distinct type from
// internal/features/service-order.Response (AC9), containing only what
// requirements.md §3.4/AC6 allows.
type trackingVehicleDTO struct {
	LicensePlate string `json:"licensePlate"`
	Brand        string `json:"brand"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Color        string `json:"color"`
}

type trackingMilestoneDTO struct {
	Event          string    `json:"event"`
	PreviousStatus string    `json:"previousStatus"`
	NewStatus      string    `json:"newStatus"`
	OccurredAt     time.Time `json:"occurredAt"`
}

type trackingResponse struct {
	Code       int64                  `json:"code"`
	Status     string                 `json:"status"`
	Vehicle    trackingVehicleDTO     `json:"vehicle"`
	Milestones []trackingMilestoneDTO `json:"milestones"`
}

func toTrackingResponse(read *trackingRead) trackingResponse {
	milestones := make([]trackingMilestoneDTO, 0, len(read.Milestones))
	for _, milestone := range read.Milestones {
		milestones = append(milestones, trackingMilestoneDTO{
			Event:          milestone.Event,
			PreviousStatus: milestone.PreviousStatus,
			NewStatus:      milestone.NewStatus,
			OccurredAt:     milestone.OccurredAt,
		})
	}

	return trackingResponse{
		Code:   read.Code,
		Status: read.Status,
		Vehicle: trackingVehicleDTO{
			LicensePlate: read.Vehicle.LicensePlate,
			Brand:        read.Vehicle.Brand,
			Model:        read.Vehicle.Model,
			Year:         read.Vehicle.Year,
			Color:        read.Vehicle.Color,
		},
		Milestones: milestones,
	}
}
