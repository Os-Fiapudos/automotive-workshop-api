package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status is a ServiceOrder's current stage in its lifecycle. Values are in
// English like every other domain identifier (changed on 2026-08-26, from
// RECEBIDA/EM_DIAGNOSTICO/AGUARDANDO_APROVACAO/EM_EXECUCAO/FINALIZADA/
// ENTREGUE/CANCELADA) — see docs/entities.md.
type Status string

// Status values. StatusReceived is the only one Service Order Opening
// produces; StatusInDiagnosis is introduced by the Diagnosis and Quote
// Composition feature (see StartDiagnosis below).
// StatusAwaitingApproval/StatusInProgress/StatusCanceled are produced by
// specs/service-order-quote-decision/ (see SendQuote/ApproveQuote/
// RejectQuote below) — until that feature, IN_PROGRESS itself was only an
// external precondition specs/service-order-execution/ depended on but did
// not create (that spec's requirements.md §2.1). StatusCompleted/
// StatusDelivered are produced by specs/service-order-execution/ — see
// Finalize/Deliver below.
const (
	StatusReceived         Status = "RECEIVED"
	StatusInDiagnosis      Status = "IN_DIAGNOSIS"
	StatusAwaitingApproval Status = "AWAITING_APPROVAL"
	StatusInProgress       Status = "IN_PROGRESS"
	StatusCompleted        Status = "COMPLETED"
	StatusDelivered        Status = "DELIVERED"
	StatusCanceled         Status = "CANCELED"
)

// knownStatusValues lists every value docs/entities.md's ServiceOrderStatus
// enum documents — used to validate the GET /api/v1/service-orders "status"
// filter (specs/service-order-query/).
var knownStatusValues = []string{
	string(StatusReceived),
	string(StatusInDiagnosis),
	string(StatusAwaitingApproval),
	string(StatusInProgress),
	string(StatusCompleted),
	string(StatusDelivered),
	string(StatusCanceled),
}

// IsKnownStatus reports whether value is one of knownStatusValues.
func IsKnownStatus(value string) bool {
	for _, known := range knownStatusValues {
		if value == known {
			return true
		}
	}
	return false
}

// ServiceOrder is the domain aggregate for this feature. It cannot be
// constructed in any status other than RECEIVED — there is no setter for
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
	RequestedProducts   []RequestedProduct
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// RequestedProduct is one product/part the customer asked for when the order
// was opened (specs/service-order-opening/requirements.md §7a) — unpriced
// initial demand, never a stock reservation or debit (§7a.1).
type RequestedProduct struct {
	ProductID uuid.UUID
	Quantity  int
}

// NewServiceOrder builds a new service order, always starting RECEIVED (see
// specs/service-order-opening/requirements.md §3.6). It only validates that
// customerID/vehicleID are present — existence, active status, and
// ownership are the use-case layer's responsibility (design.md §1.2), since
// they require repository access this constructor does not have.
//
// requestedServiceIDs and requestedProducts may be empty: no requirement
// mandates at least one requested item at opening time. Every requested
// product must have a positive quantity and appear at most once
// (specs/service-order-opening/design.md §7.1).
func NewServiceOrder(customerID, vehicleID uuid.UUID, notes string, requestedServiceIDs []uuid.UUID, requestedProducts []RequestedProduct) (*ServiceOrder, error) {
	if customerID == uuid.Nil || vehicleID == uuid.Nil {
		return nil, ErrInvalidAggregate
	}

	seenProducts := make(map[uuid.UUID]bool, len(requestedProducts))
	for _, product := range requestedProducts {
		if product.Quantity <= 0 {
			return nil, ErrInvalidRequestedProductQuantity
		}
		if seenProducts[product.ProductID] {
			return nil, ErrDuplicateRequestedProduct
		}
		seenProducts[product.ProductID] = true
	}

	return &ServiceOrder{
		ID:                  uuid.New(),
		CustomerID:          customerID,
		VehicleID:           vehicleID,
		Status:              StatusReceived,
		Notes:               notes,
		RequestedServiceIDs: requestedServiceIDs,
		RequestedProducts:   requestedProducts,
	}, nil
}

// StartDiagnosis transitions the order from RECEIVED to IN_DIAGNOSIS
// (specs/service-order-diagnosis-quote/requirements.md §3.1). The transition
// methods below are the only way Status changes after construction — there
// is still no setter.
func (order *ServiceOrder) StartDiagnosis() error {
	if order.Status != StatusReceived {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusInDiagnosis
	return nil
}

// SendQuote transitions the order from IN_DIAGNOSIS to
// AWAITING_APPROVAL once its composed quote has been sent to the customer
// (specs/service-order-quote-decision/requirements.md — "o envio altera a OS
// de IN_DIAGNOSIS para AWAITING_APPROVAL"). Composing/recomposing the
// quote itself (ComposeQuote) no longer performs this transition — only
// sending it does.
func (order *ServiceOrder) SendQuote() error {
	if order.Status != StatusInDiagnosis {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusAwaitingApproval
	return nil
}

// ApproveQuote transitions the order from AWAITING_APPROVAL to
// IN_PROGRESS once the customer approves its quote
// (specs/service-order-quote-decision/requirements.md — "a aprovação altera
// automaticamente a OS para IN_PROGRESS"). This is the transition
// specs/service-order-execution/requirements.md §2.1 flagged as depended on
// but not produced by any code until this feature.
func (order *ServiceOrder) ApproveQuote() error {
	if order.Status != StatusAwaitingApproval {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusInProgress
	return nil
}

// RejectQuote transitions the order from AWAITING_APPROVAL to CANCELED
// once the customer rejects its quote — the closing status decided for a
// rejected quote (specs/service-order-quote-decision/requirements.md), since
// a REJECTED quote can never be altered
// (specs/service-order-diagnosis-quote/requirements.md §3.9) and the order
// would otherwise have no way to leave AWAITING_APPROVAL.
func (order *ServiceOrder) RejectQuote() error {
	if order.Status != StatusAwaitingApproval {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusCanceled
	return nil
}

// Finalize transitions the order from IN_PROGRESS to COMPLETED
// (specs/service-order-execution/requirements.md §4, BR5 — the use-case
// layer checks the required-executions rule before calling this).
func (order *ServiceOrder) Finalize() error {
	if order.Status != StatusInProgress {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusCompleted
	return nil
}

// Deliver transitions the order from COMPLETED to DELIVERED
// (specs/service-order-execution/requirements.md §4, BR7).
func (order *ServiceOrder) Deliver() error {
	if order.Status != StatusCompleted {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusDelivered
	return nil
}
