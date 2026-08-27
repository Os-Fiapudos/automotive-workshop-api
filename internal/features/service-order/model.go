package serviceorder

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
// Composition feature (see startDiagnosis below).
// StatusAwaitingApproval/StatusInProgress/StatusCanceled are produced by
// specs/service-order-quote-decision/ (see sendQuote/approveQuote/
// rejectQuote below) — until that feature, IN_PROGRESS itself was only an
// external precondition specs/service-order-execution/ depended on but did
// not create (that spec's requirements.md §2.1). StatusCompleted/
// StatusDelivered are produced by specs/service-order-execution/ — see
// finalize/deliver below.
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

// isKnownStatus reports whether value is one of knownStatusValues.
func isKnownStatus(value string) bool {
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
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NewServiceOrder builds a new service order, always starting RECEIVED (see
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
		Status:              StatusReceived,
		Notes:               notes,
		RequestedServiceIDs: requestedServiceIDs,
	}, nil
}

// startDiagnosis transitions the order from RECEIVED to IN_DIAGNOSIS
// (specs/service-order-diagnosis-quote/requirements.md §3.1). It is the only
// way Status ever changes after construction — there is still no exported
// setter.
func (order *ServiceOrder) startDiagnosis() error {
	if order.Status != StatusReceived {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusInDiagnosis
	return nil
}

// sendQuote transitions the order from IN_DIAGNOSIS to
// AWAITING_APPROVAL once its composed quote has been sent to the customer
// (specs/service-order-quote-decision/requirements.md — "o envio altera a OS
// de IN_DIAGNOSIS para AWAITING_APPROVAL"). Composing/recomposing the
// quote itself (ComposeQuote) no longer performs this transition — only
// sending it does.
func (order *ServiceOrder) sendQuote() error {
	if order.Status != StatusInDiagnosis {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusAwaitingApproval
	return nil
}

// approveQuote transitions the order from AWAITING_APPROVAL to
// IN_PROGRESS once the customer approves its quote
// (specs/service-order-quote-decision/requirements.md — "a aprovação altera
// automaticamente a OS para IN_PROGRESS"). This is the transition
// specs/service-order-execution/requirements.md §2.1 flagged as depended on
// but not produced by any code until this feature.
func (order *ServiceOrder) approveQuote() error {
	if order.Status != StatusAwaitingApproval {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusInProgress
	return nil
}

// rejectQuote transitions the order from AWAITING_APPROVAL to CANCELED
// once the customer rejects its quote — the closing status decided for a
// rejected quote (specs/service-order-quote-decision/requirements.md), since
// a REJECTED quote can never be altered
// (specs/service-order-diagnosis-quote/requirements.md §3.9) and the order
// would otherwise have no way to leave AWAITING_APPROVAL.
func (order *ServiceOrder) rejectQuote() error {
	if order.Status != StatusAwaitingApproval {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusCanceled
	return nil
}

// finalize transitions the order from IN_PROGRESS to COMPLETED
// (specs/service-order-execution/requirements.md §4, BR5 — the service layer
// checks the required-executions rule before calling this).
func (order *ServiceOrder) finalize() error {
	if order.Status != StatusInProgress {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusCompleted
	return nil
}

// deliver transitions the order from COMPLETED to DELIVERED
// (specs/service-order-execution/requirements.md §4, BR7).
func (order *ServiceOrder) deliver() error {
	if order.Status != StatusCompleted {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusDelivered
	return nil
}

// QuoteStatus is a Quote's decision status, in English like every other
// domain enum — see docs/entities.md.
type QuoteStatus string

const (
	QuoteStatusPending  QuoteStatus = "PENDING"
	QuoteStatusApproved QuoteStatus = "APPROVED"
	QuoteStatusRejected QuoteStatus = "REJECTED"
)

// QuoteItemKind distinguishes a product/part line from a service line within
// a Quote.
type QuoteItemKind string

const (
	QuoteItemProduct QuoteItemKind = "PRODUCT"
	QuoteItemService QuoteItemKind = "SERVICE"
)

// QuoteItemInput is a raw, unresolved line item as supplied by a compose
// request — mirrors CreateInput's "raw strings in, resolved refs out" shape
// (see ServiceOrderService.Create).
type QuoteItemInput struct {
	Kind      QuoteItemKind
	ProductID string
	ServiceID string
	Quantity  int
}

// QuoteItem is a resolved, priced quote line — the snapshot that gets
// persisted. Description and UnitPrice are copied from the catalog at
// composition time and never change afterward, even if the catalog does
// (requirements.md §3.5).
type QuoteItem struct {
	Kind        QuoteItemKind
	ProductID   uuid.UUID
	ServiceID   uuid.UUID
	Description string
	Quantity    int
	UnitPrice   float64
	Total       float64
}

// Quote is the priced budget for a ServiceOrder, composed from diagnosed
// items (requirements.md, RF05/RF06). Version/SentAt/SentVersion are added by
// specs/service-order-quote-decision/: Version increments on every
// compose/recompose (specs/service-order-diagnosis-quote/); SentAt/
// SentVersion record when the quote was sent to the customer and which
// Version was actually presented, satisfying that feature's requirement to
// register "a data de envio e a versão efetivamente apresentada".
type Quote struct {
	ID             uuid.UUID
	Code           int64
	ServiceOrderID uuid.UUID
	TotalAmount    float64
	Status         QuoteStatus
	Version        int
	Items          []QuoteItem
	GeneratedAt    time.Time
	SentAt         *time.Time
	SentVersion    *int
	RespondedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// validateQuoteItems enforces requirements.md §3.3/§3.6: at least one item,
// every quantity strictly positive.
func validateQuoteItems(items []QuoteItem) error {
	if len(items) == 0 {
		return ErrEmptyQuote
	}
	for _, item := range items {
		if item.Quantity <= 0 {
			return ErrInvalidQuantity
		}
	}
	return nil
}

// calculateTotal sums every item's Total. This is the single place RF06
// ("the total must be calculated exclusively by the back end") is
// implemented — no other code path computes a quote total.
func calculateTotal(items []QuoteItem) float64 {
	var total float64
	for _, item := range items {
		total += item.Total
	}
	return total
}

// ServiceOrderHistory is a single audit-trail entry of a ServiceOrder's
// status changes (docs/entities.md), read-only from this package's
// perspective — every row is written by service-order-opening (the
// "creation" event) or service-order-diagnosis-quote ("diagnosis_started",
// "quote_composed"); specs/service-order-query/ only ever reads them back.
type ServiceOrderHistory struct {
	ID             uuid.UUID
	ServiceOrderID uuid.UUID
	OccurredAt     time.Time
	Event          string
	Description    string
	PreviousStatus Status
	NewStatus      Status
}
