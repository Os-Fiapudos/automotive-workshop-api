package serviceorder

import (
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// CreateRequest is the POST /api/v1/service-orders request body. Exactly one
// of CustomerID/CustomerDocument and exactly one of VehicleID/LicensePlate
// must be present — see Validate. Any status field the client sends is
// intentionally not part of this struct at all: the server is the only
// author of a new order's status (requirements.md §3.6/§8).
type CreateRequest struct {
	CustomerID          string   `json:"customerId,omitempty"`
	CustomerDocument    string   `json:"customerDocument,omitempty"`
	VehicleID           string   `json:"vehicleId,omitempty"`
	LicensePlate        string   `json:"licensePlate,omitempty"`
	RequestedServiceIDs []string `json:"requestedServiceIds,omitempty"`
	Notes               string   `json:"notes,omitempty"`
}

// Validate checks that the request identifies exactly one customer and
// exactly one vehicle. Deeper validation (does the customer/vehicle/service
// actually exist and is it usable) happens in the service layer, which is
// the only layer with repository access.
func (request CreateRequest) Validate() []apierror.Detail {
	var details []apierror.Detail

	hasCustomerID := strings.TrimSpace(request.CustomerID) != ""
	hasCustomerDocument := strings.TrimSpace(request.CustomerDocument) != ""
	if hasCustomerID == hasCustomerDocument {
		details = append(details, apierror.Detail{Field: "customerId", Message: "exactly one of customerId or customerDocument is required"})
	}

	hasVehicleID := strings.TrimSpace(request.VehicleID) != ""
	hasLicensePlate := strings.TrimSpace(request.LicensePlate) != ""
	if hasVehicleID == hasLicensePlate {
		details = append(details, apierror.Detail{Field: "vehicleId", Message: "exactly one of vehicleId or licensePlate is required"})
	}

	return details
}

func (request CreateRequest) toInput() CreateInput {
	return CreateInput{
		CustomerID:          request.CustomerID,
		CustomerDocument:    request.CustomerDocument,
		VehicleID:           request.VehicleID,
		LicensePlate:        request.LicensePlate,
		RequestedServiceIDs: request.RequestedServiceIDs,
		Notes:               request.Notes,
	}
}

// customerSummary/vehicleSummary/serviceSummary are the response's nested
// shapes — deliberately not the full Customer/Vehicle/Service payloads
// (design.md §4.2).
type customerSummary struct {
	ID   string `json:"id"`
	Code int64  `json:"code"`
	Name string `json:"name"`
}

type vehicleSummary struct {
	ID           string `json:"id"`
	Code         int64  `json:"code"`
	LicensePlate string `json:"licensePlate"`
}

type serviceSummary struct {
	ID   string `json:"id"`
	Code int64  `json:"code"`
	Name string `json:"name"`
}

// Response is the JSON representation of a created ServiceOrder.
// TrackingToken is the raw customer-tracking token generated for this order
// (specs/service-order-tracking/design.md §5) — it exists only in this
// creation response; it is never returned again and never logged (RNF08).
type Response struct {
	ID                string           `json:"id"`
	Code              int64            `json:"code"`
	Customer          customerSummary  `json:"customer"`
	Vehicle           vehicleSummary   `json:"vehicle"`
	OpenedAt          time.Time        `json:"openedAt"`
	Status            string           `json:"status"`
	Notes             string           `json:"notes"`
	RequestedServices []serviceSummary `json:"requestedServices"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
	TrackingToken     string           `json:"trackingToken"`
}

func toResponse(result *CreateResult) Response {
	requestedServices := make([]serviceSummary, 0, len(result.Services))
	for _, service := range result.Services {
		requestedServices = append(requestedServices, serviceSummary{
			ID:   service.ID.String(),
			Code: service.Code,
			Name: service.Name,
		})
	}

	return Response{
		ID:   result.Order.ID.String(),
		Code: result.Order.Code,
		Customer: customerSummary{
			ID:   result.Customer.ID.String(),
			Code: result.Customer.Code,
			Name: result.Customer.Name,
		},
		Vehicle: vehicleSummary{
			ID:           result.Vehicle.ID.String(),
			Code:         result.Vehicle.Code,
			LicensePlate: result.Vehicle.LicensePlate,
		},
		OpenedAt:          result.Order.OpenedAt,
		Status:            string(result.Order.Status),
		Notes:             result.Order.Notes,
		RequestedServices: requestedServices,
		CreatedAt:         result.Order.CreatedAt,
		UpdatedAt:         result.Order.UpdatedAt,
		TrackingToken:     result.TrackingToken,
	}
}
