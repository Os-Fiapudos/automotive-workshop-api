package usecase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
)

// MaxStatusUpdateNotesLength caps the optional notes an external status
// update may attach to its history entry, counted in characters after
// trimming (specs/service-order-status-update/requirements.md BR6).
const MaxStatusUpdateNotesLength = 500

// externalStatusTargets is the closed set of statuses the external
// status-update channel may request (requirements.md BR2, §7.1). Anything
// else — including AWAITING_APPROVAL/IN_PROGRESS, so quote approval can
// never be skipped — is domain.ErrUnsupportedExternalStatus.
var externalStatusTargets = []domain.Status{domain.StatusInDiagnosis, domain.StatusCompleted, domain.StatusDelivered}

// IsExternalStatusTarget reports whether status is one of externalStatusTargets.
func IsExternalStatusTarget(status domain.Status) bool {
	for _, target := range externalStatusTargets {
		if status == target {
			return true
		}
	}
	return false
}

// StatusUpdateResult is the outcome of UpdateStatusExternal: the order after
// the transition plus the status it had before, for the reduced response
// (specs/service-order-status-update/requirements.md BR8).
type StatusUpdateResult struct {
	Order          *domain.ServiceOrder
	PreviousStatus domain.Status
}

// Lifecycle groups the status transitions that write only the order and its
// history entry: starting diagnosis (specs/service-order-diagnosis-quote/),
// finalizing and delivering (specs/service-order-execution/), and the
// external status update that dispatches onto those same cores
// (specs/service-order-status-update/). externalStatusUpdateSecret
// authenticates that external channel (design.md §3) — set once at wiring
// time via WithExternalStatusUpdateSecret; left empty, that channel rejects
// every caller.
type Lifecycle struct {
	orders                     OrderFinder
	transitions                StatusTransitionStore
	quotes                     QuoteReader
	executions                 ExecutionLister
	externalStatusUpdateSecret string
}

func NewLifecycle(orders OrderFinder, transitions StatusTransitionStore, quotes QuoteReader, executions ExecutionLister) *Lifecycle {
	return &Lifecycle{orders: orders, transitions: transitions, quotes: quotes, executions: executions}
}

// WithExternalStatusUpdateSecret sets the shared secret the external
// status-update endpoint authenticates against
// (specs/service-order-status-update/design.md §3) and returns the same use
// case, so it chains onto NewLifecycle at wiring time.
func (lifecycle *Lifecycle) WithExternalStatusUpdateSecret(secret string) *Lifecycle {
	lifecycle.externalStatusUpdateSecret = secret
	return lifecycle
}

// StartDiagnosis transitions a service order from RECEIVED to
// IN_DIAGNOSIS (specs/service-order-diagnosis-quote/requirements.md §3.1).
func (lifecycle *Lifecycle) StartDiagnosis(ctx context.Context, serviceOrderID uuid.UUID) (*domain.ServiceOrder, error) {
	order, _, err := lifecycle.startDiagnosis(ctx, serviceOrderID, "")
	return order, err
}

// startDiagnosis is the single core behind StartDiagnosis and the external
// status update (specs/service-order-status-update/design.md §1): notes, if
// non-empty, is appended to the history entry. It also returns the status
// the order had before the transition.
func (lifecycle *Lifecycle) startDiagnosis(ctx context.Context, serviceOrderID uuid.UUID, notes string) (*domain.ServiceOrder, domain.Status, error) {
	order, err := lifecycle.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, "", err
	}
	previousStatus := order.Status

	if err := order.StartDiagnosis(); err != nil {
		return nil, "", err
	}

	if err := lifecycle.transitions.StartDiagnosis(ctx, order, notes); err != nil {
		return nil, "", err
	}

	return order, previousStatus, nil
}

// FinalizeOrder transitions the order to COMPLETED once every required
// execution — one per distinct service line item of its approved quote — is
// complete (specs/service-order-execution/ BR5, design.md §2.3).
func (lifecycle *Lifecycle) FinalizeOrder(ctx context.Context, serviceOrderID uuid.UUID) (*domain.ServiceOrder, error) {
	order, _, err := lifecycle.finalizeOrder(ctx, serviceOrderID, "")
	return order, err
}

// finalizeOrder is the single core behind FinalizeOrder and the external
// status update (specs/service-order-status-update/design.md §1): notes, if
// non-empty, is appended to the history entry. It also returns the status
// the order had before the transition.
func (lifecycle *Lifecycle) finalizeOrder(ctx context.Context, serviceOrderID uuid.UUID, notes string) (*domain.ServiceOrder, domain.Status, error) {
	order, err := lifecycle.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, "", err
	}
	if order.Status != domain.StatusInProgress {
		return nil, "", domain.ErrInvalidStatusTransition
	}
	previousStatus := order.Status

	quote, err := lifecycle.quotes.FindQuoteByServiceOrderID(ctx, serviceOrderID)
	if err != nil && !errors.Is(err, domain.ErrQuoteNotFound) {
		return nil, "", err
	}

	if quote != nil {
		if err := lifecycle.ensureRequiredExecutionsComplete(ctx, order.ID, quote); err != nil {
			return nil, "", err
		}
	}

	if err := order.Finalize(); err != nil {
		return nil, "", err
	}
	if err := lifecycle.transitions.FinalizeOrder(ctx, order, notes); err != nil {
		return nil, "", err
	}
	return order, previousStatus, nil
}

// ensureRequiredExecutionsComplete implements BR5: every distinct service id
// among quote's QuoteItemService line items must have at least one execution
// with a recorded end date.
func (lifecycle *Lifecycle) ensureRequiredExecutionsComplete(ctx context.Context, serviceOrderID uuid.UUID, quote *domain.Quote) error {
	required := make(map[uuid.UUID]bool)
	for _, item := range quote.Items {
		if item.Kind == domain.QuoteItemService {
			required[item.ServiceID] = true
		}
	}
	if len(required) == 0 {
		return nil
	}

	executions, err := lifecycle.executions.FindServiceExecutionsByServiceOrderID(ctx, serviceOrderID)
	if err != nil {
		return err
	}
	completed := make(map[uuid.UUID]bool)
	for _, execution := range executions {
		if execution.EndedAt != nil {
			completed[execution.ServiceID] = true
		}
	}

	for serviceID := range required {
		if !completed[serviceID] {
			return domain.ErrExecutionsNotCompleted
		}
	}
	return nil
}

// DeliverOrder transitions a COMPLETED order to DELIVERED
// (specs/service-order-execution/ BR7).
func (lifecycle *Lifecycle) DeliverOrder(ctx context.Context, serviceOrderID uuid.UUID) (*domain.ServiceOrder, error) {
	order, _, err := lifecycle.deliverOrder(ctx, serviceOrderID, "")
	return order, err
}

// deliverOrder is the single core behind DeliverOrder and the external
// status update (specs/service-order-status-update/design.md §1): notes, if
// non-empty, is appended to the history entry. It also returns the status
// the order had before the transition.
func (lifecycle *Lifecycle) deliverOrder(ctx context.Context, serviceOrderID uuid.UUID, notes string) (*domain.ServiceOrder, domain.Status, error) {
	order, err := lifecycle.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, "", err
	}
	previousStatus := order.Status

	if err := order.Deliver(); err != nil {
		return nil, "", err
	}
	if err := lifecycle.transitions.DeliverOrder(ctx, order, notes); err != nil {
		return nil, "", err
	}
	return order, previousStatus, nil
}

// UpdateStatusExternal performs a transition requested by the external
// status-update tool (specs/service-order-status-update/design.md §1). It is
// a pure dispatch onto the same cores the administrative /diagnosis,
// /finalize and /deliver endpoints use — same lookup, status guard and
// transaction — so it adds no transition rule of its own (BR1, BR3). The
// caller must already have been authenticated (AuthenticateExternalStatusUpdate).
func (lifecycle *Lifecycle) UpdateStatusExternal(ctx context.Context, serviceOrderID uuid.UUID, targetStatus domain.Status, notes string) (*StatusUpdateResult, error) {
	if !IsExternalStatusTarget(targetStatus) {
		return nil, domain.ErrUnsupportedExternalStatus
	}
	notes, err := normalizeStatusUpdateNotes(notes)
	if err != nil {
		return nil, err
	}

	var order *domain.ServiceOrder
	var previousStatus domain.Status
	switch targetStatus {
	case domain.StatusInDiagnosis:
		order, previousStatus, err = lifecycle.startDiagnosis(ctx, serviceOrderID, notes)
	case domain.StatusCompleted:
		order, previousStatus, err = lifecycle.finalizeOrder(ctx, serviceOrderID, notes)
	case domain.StatusDelivered:
		order, previousStatus, err = lifecycle.deliverOrder(ctx, serviceOrderID, notes)
	}
	if err != nil {
		return nil, err
	}
	return &StatusUpdateResult{Order: order, PreviousStatus: previousStatus}, nil
}

// normalizeStatusUpdateNotes trims notes and enforces
// MaxStatusUpdateNotesLength (BR6). Blank notes become "" (absent).
func normalizeStatusUpdateNotes(notes string) (string, error) {
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > MaxStatusUpdateNotesLength {
		return "", domain.ErrInvalidStatusUpdateNotes
	}
	return notes, nil
}

// AuthenticateExternalStatusUpdate reports whether presented matches the
// configured external status-update secret (design.md §3). It is a
// different secret from the quote-decision one (BR4); an unconfigured
// (empty) secret never authenticates anyone.
func (lifecycle *Lifecycle) AuthenticateExternalStatusUpdate(presented string) bool {
	return secretMatches(lifecycle.externalStatusUpdateSecret, presented)
}
