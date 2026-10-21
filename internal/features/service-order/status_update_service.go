package serviceorder

import (
	"automotive-workshop-api/internal/features/service-order/domain"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxStatusUpdateNotesLength caps the optional notes an external status
// update may attach to its history entry, counted in characters after
// trimming (specs/service-order-status-update/requirements.md BR6).
const maxStatusUpdateNotesLength = 500

// externalStatusTargets is the closed set of statuses the external
// status-update channel may request (requirements.md BR2, §7.1). Anything
// else — including AWAITING_APPROVAL/IN_PROGRESS, so quote approval can
// never be skipped — is ErrUnsupportedExternalStatus.
var externalStatusTargets = []domain.Status{domain.StatusInDiagnosis, domain.StatusCompleted, domain.StatusDelivered}

// isExternalStatusTarget reports whether status is one of externalStatusTargets.
func isExternalStatusTarget(status domain.Status) bool {
	for _, target := range externalStatusTargets {
		if status == target {
			return true
		}
	}
	return false
}

// StatusUpdateResult is the outcome of UpdateStatusExternal: the order after
// the transition plus the status it had before, for the reduced response
// (requirements.md BR8).
type StatusUpdateResult struct {
	Order          *domain.ServiceOrder
	PreviousStatus domain.Status
}

// UpdateStatusExternal performs a transition requested by the external
// status-update tool (specs/service-order-status-update/design.md §1). It is
// a pure dispatch onto the same cores the administrative /diagnosis,
// /finalize and /deliver endpoints use — same lookup, status guard and
// transaction — so it adds no transition rule of its own (BR1, BR3). The
// caller must already have been authenticated (AuthenticateExternalStatusUpdate).
func (service *ServiceOrderService) UpdateStatusExternal(ctx context.Context, serviceOrderID uuid.UUID, targetStatus domain.Status, notes string) (*StatusUpdateResult, error) {
	if !isExternalStatusTarget(targetStatus) {
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
		order, previousStatus, err = service.startDiagnosis(ctx, serviceOrderID, notes)
	case domain.StatusCompleted:
		order, previousStatus, err = service.finalizeOrder(ctx, serviceOrderID, notes)
	case domain.StatusDelivered:
		order, previousStatus, err = service.deliverOrder(ctx, serviceOrderID, notes)
	}
	if err != nil {
		return nil, err
	}
	return &StatusUpdateResult{Order: order, PreviousStatus: previousStatus}, nil
}

// normalizeStatusUpdateNotes trims notes and enforces
// maxStatusUpdateNotesLength (BR6). Blank notes become "" (absent).
func normalizeStatusUpdateNotes(notes string) (string, error) {
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > maxStatusUpdateNotesLength {
		return "", domain.ErrInvalidStatusUpdateNotes
	}
	return notes, nil
}

// AuthenticateExternalStatusUpdate reports whether presented matches the
// configured external status-update secret (design.md §3). It is a
// different secret from the quote-decision one (BR4); an unconfigured
// (empty) secret never authenticates anyone.
func (service *ServiceOrderService) AuthenticateExternalStatusUpdate(presented string) bool {
	return secretMatches(service.externalStatusUpdateSecret, presented)
}

// secretMatches compares a presented shared secret with the configured one.
// Both are hashed before the constant-time comparison so neither the
// content nor the length of the secret leaks through timing. An empty
// configured or presented value never matches.
func secretMatches(configured, presented string) bool {
	if configured == "" || presented == "" {
		return false
	}
	expected := sha256.Sum256([]byte(configured))
	actual := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
}
