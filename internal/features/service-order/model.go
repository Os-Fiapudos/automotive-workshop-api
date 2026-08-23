package serviceorder

import (
	"time"

	"github.com/google/uuid"
)

// Status is a ServiceOrder's current stage in its lifecycle. Values are kept
// in Portuguese by explicit product decision — see docs/entities.md.
type Status string

// Status values. StatusRecebida is the only one Service Order Opening
// produces; StatusEmDiagnostico/StatusAguardandoAprovacao are introduced by
// the Diagnosis and Quote Composition feature (see startDiagnosis/
// markAwaitingApproval below). StatusEmExecucao/StatusFinalizada/
// StatusEntregue are consumed (not produced) by
// specs/service-order-execution/ — see finalize/deliver below and that
// spec's requirements.md §2.1 for why reaching EM_EXECUCAO itself is still
// an external precondition, not a transition this codebase implements yet.
const (
	StatusRecebida            Status = "RECEBIDA"
	StatusEmDiagnostico       Status = "EM_DIAGNOSTICO"
	StatusAguardandoAprovacao Status = "AGUARDANDO_APROVACAO"
	StatusEmExecucao          Status = "EM_EXECUCAO"
	StatusFinalizada          Status = "FINALIZADA"
	StatusEntregue            Status = "ENTREGUE"
)

// knownStatusValues lists every value docs/entities.md's ServiceOrderStatus
// enum documents — used to validate the GET /api/v1/service-orders "status"
// filter (specs/service-order-query/).
var knownStatusValues = []string{
	string(StatusRecebida),
	string(StatusEmDiagnostico),
	string(StatusAguardandoAprovacao),
	string(StatusEmExecucao),
	string(StatusFinalizada),
	string(StatusEntregue),
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

// startDiagnosis transitions the order from RECEBIDA to EM_DIAGNOSTICO
// (specs/service-order-diagnosis-quote/requirements.md §3.1). It is the only
// way Status ever changes after construction — there is still no exported
// setter.
func (order *ServiceOrder) startDiagnosis() error {
	if order.Status != StatusRecebida {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusEmDiagnostico
	return nil
}

// markAwaitingApproval transitions the order to AGUARDANDO_APROVACAO once a
// quote has been composed (requirements.md §3.2, §3.9). Diagnosis must have
// already started (Status != RECEBIDA); re-entering AGUARDANDO_APROVACAO
// from itself is allowed and a no-op, since composing a quote is idempotent
// while it is still PENDING.
func (order *ServiceOrder) markAwaitingApproval() error {
	if order.Status == StatusRecebida {
		return ErrDiagnosisNotStarted
	}
	order.Status = StatusAguardandoAprovacao
	return nil
}

// finalize transitions the order from EM_EXECUCAO to FINALIZADA
// (specs/service-order-execution/requirements.md §4, BR5 — the service layer
// checks the required-executions rule before calling this).
func (order *ServiceOrder) finalize() error {
	if order.Status != StatusEmExecucao {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusFinalizada
	return nil
}

// deliver transitions the order from FINALIZADA to ENTREGUE
// (specs/service-order-execution/requirements.md §4, BR7).
func (order *ServiceOrder) deliver() error {
	if order.Status != StatusFinalizada {
		return ErrInvalidStatusTransition
	}
	order.Status = StatusEntregue
	return nil
}

// QuoteStatus is a Quote's decision status. Kept in English, unlike
// ServiceOrder.Status — see docs/entities.md's note on the single deliberate
// Portuguese exception.
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
// items (requirements.md, RF05/RF06).
type Quote struct {
	ID             uuid.UUID
	Code           int64
	ServiceOrderID uuid.UUID
	TotalAmount    float64
	Status         QuoteStatus
	Items          []QuoteItem
	GeneratedAt    time.Time
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
