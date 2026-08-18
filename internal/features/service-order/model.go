package serviceorder

import (
	"time"

	"github.com/google/uuid"
)

// Status is a ServiceOrder's current stage in its lifecycle. Values are kept
// in Portuguese by explicit product decision — see docs/entities.md.
type Status string

// StatusRecebida is the only status this feature ever produces. Later
// transitions (EM_DIAGNOSTICO, AGUARDANDO_APROVACAO, EM_EXECUCAO, FINALIZADA,
// ENTREGUE) belong to future features, not to Service Order Opening.
const StatusRecebida Status = "RECEBIDA"

// ServiceOrder is the domain aggregate for this feature. It cannot be
// constructed in any status other than RECEBIDA — there is no setter for
// Status and no other constructor (requirements.md §3.6).
type ServiceOrder struct {
	ID                  uuid.UUID
	Code                int64
	CustomerID          uuid.UUID
	VehicleID           uuid.UUID
	OpenedAt            time.Time
	Status              Status
	Notes               string
	RequestedServiceIDs []uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NewServiceOrder builds a new service order, always starting RECEBIDA (see
// specs/service-order-opening/requirements.md §3.6). It only validates that
// customerID/vehicleID are present — existence, active status, and
// ownership are the service layer's responsibility (design.md §1.2), since
// they require repository access this constructor does not have.
//
// requestedServiceIDs may be empty: no requirement mandates at least one
// requested service at opening time.
func NewServiceOrder(customerID, vehicleID uuid.UUID, notes string, requestedServiceIDs []uuid.UUID) (*ServiceOrder, error) {
	if customerID == uuid.Nil || vehicleID == uuid.Nil {
		return nil, ErrInvalidAggregate
	}

	return &ServiceOrder{
		ID:                  uuid.New(),
		CustomerID:          customerID,
		VehicleID:           vehicleID,
		Status:              StatusRecebida,
		Notes:               notes,
		RequestedServiceIDs: requestedServiceIDs,
	}, nil
}
