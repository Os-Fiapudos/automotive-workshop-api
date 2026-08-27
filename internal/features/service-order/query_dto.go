package serviceorder

import "time"

// listItemResponse is one row of the GET /api/v1/service-orders response
// body — deliberately lighter than DetailResponse (no items/history), same
// summary-vs-full-payload split design.md §1.9 documents.
type listItemResponse struct {
	ID        string          `json:"id"`
	Code      int64           `json:"code"`
	Customer  customerSummary `json:"customer"`
	Vehicle   vehicleSummary  `json:"vehicle"`
	Status    string          `json:"status"`
	OpenedAt  time.Time       `json:"openedAt"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// ListResponse is the GET /api/v1/service-orders response body — the
// data/page/pageSize/total/totalPages envelope customer/vehicle/product
// already use (design.md §1.5).
type ListResponse struct {
	Data       []listItemResponse `json:"data"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
	Total      int                `json:"total"`
	TotalPages int                `json:"totalPages"`
}

func toListResponse(items []*ServiceOrderListItem, page, pageSize, total int) ListResponse {
	data := make([]listItemResponse, 0, len(items))
	for _, item := range items {
		data = append(data, listItemResponse{
			ID:   item.Order.ID.String(),
			Code: item.Order.Code,
			Customer: customerSummary{
				ID:   item.Customer.ID.String(),
				Code: item.Customer.Code,
				Name: item.Customer.Name,
			},
			Vehicle: vehicleSummary{
				ID:           item.Vehicle.ID.String(),
				Code:         item.Vehicle.Code,
				LicensePlate: item.Vehicle.LicensePlate,
			},
			Status:    string(item.Order.Status),
			OpenedAt:  item.Order.OpenedAt,
			CreatedAt: item.Order.CreatedAt,
			UpdatedAt: item.Order.UpdatedAt,
		})
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	return ListResponse{
		Data:       data,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

// customerDetail/vehicleDetail are the richer, detail-only projections
// (design.md §1.9) — deliberately not customerSummary/vehicleSummary, which
// the listing (and the create response) use.
type customerDetail struct {
	ID       string `json:"id"`
	Code     int64  `json:"code"`
	Name     string `json:"name"`
	Document string `json:"document"`
	Phone    string `json:"phone"`
}

type vehicleDetail struct {
	ID           string `json:"id"`
	Code         int64  `json:"code"`
	LicensePlate string `json:"licensePlate"`
	Brand        string `json:"brand"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Color        string `json:"color"`
}

// historyEntryResponse is a ServiceOrderHistory entry's JSON representation.
type historyEntryResponse struct {
	ID             string    `json:"id"`
	OccurredAt     time.Time `json:"occurredAt"`
	Event          string    `json:"event"`
	Description    string    `json:"description"`
	PreviousStatus string    `json:"previousStatus"`
	NewStatus      string    `json:"newStatus"`
}

// DetailResponse is the response body of GET /api/v1/service-orders/{id},
// whether {id} was a UUID or a sequential code (requirements.md BR4/AC8,
// design.md §1.2). Quote is omitted (not just null) when no quote has been
// composed yet.
type DetailResponse struct {
	ID                string                 `json:"id"`
	Code              int64                  `json:"code"`
	Customer          customerDetail         `json:"customer"`
	Vehicle           vehicleDetail          `json:"vehicle"`
	Status            string                 `json:"status"`
	Notes             string                 `json:"notes"`
	RequestedServices []serviceSummary       `json:"requestedServices"`
	Quote             *QuoteResponse         `json:"quote,omitempty"`
	History           []historyEntryResponse `json:"history"`
	OpenedAt          time.Time              `json:"openedAt"`
	CreatedAt         time.Time              `json:"createdAt"`
	UpdatedAt         time.Time              `json:"updatedAt"`
}

func toDetailResponse(detail *ServiceOrderDetail) DetailResponse {
	requestedServices := make([]serviceSummary, 0, len(detail.RequestedServices))
	for _, service := range detail.RequestedServices {
		requestedServices = append(requestedServices, serviceSummary{
			ID:   service.ID.String(),
			Code: service.Code,
			Name: service.Name,
		})
	}

	history := make([]historyEntryResponse, 0, len(detail.History))
	for _, entry := range detail.History {
		history = append(history, historyEntryResponse{
			ID:             entry.ID.String(),
			OccurredAt:     entry.OccurredAt,
			Event:          entry.Event,
			Description:    entry.Description,
			PreviousStatus: string(entry.PreviousStatus),
			NewStatus:      string(entry.NewStatus),
		})
	}

	var quote *QuoteResponse
	if detail.Quote != nil {
		response := toQuoteResponse(detail.Quote)
		quote = &response
	}

	return DetailResponse{
		ID:   detail.Order.ID.String(),
		Code: detail.Order.Code,
		Customer: customerDetail{
			ID:       detail.Customer.ID.String(),
			Code:     detail.Customer.Code,
			Name:     detail.Customer.Name,
			Document: detail.Customer.Document,
			Phone:    detail.Customer.Phone,
		},
		Vehicle: vehicleDetail{
			ID:           detail.Vehicle.ID.String(),
			Code:         detail.Vehicle.Code,
			LicensePlate: detail.Vehicle.LicensePlate,
			Brand:        detail.Vehicle.Brand,
			Model:        detail.Vehicle.Model,
			Year:         detail.Vehicle.Year,
			Color:        detail.Vehicle.Color,
		},
		Status:            string(detail.Order.Status),
		Notes:             detail.Order.Notes,
		RequestedServices: requestedServices,
		Quote:             quote,
		History:           history,
		OpenedAt:          detail.Order.OpenedAt,
		CreatedAt:         detail.Order.CreatedAt,
		UpdatedAt:         detail.Order.UpdatedAt,
	}
}
