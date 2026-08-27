package servicetracking

import (
	"context"
	"strings"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// TrackingRepository is the read-only persistence boundary this feature
// needs — a single lookup, kept use-case-shaped rather than a speculative
// CRUD surface (same convention as serviceorder.ServiceOrderRepository).
type TrackingRepository interface {
	// FindByCodeAndTokenHash resolves the order identified by code first
	// (so an unknown code can be reported as ErrOrderNotFound regardless of
	// the token), then checks tokenHash against that specific order's
	// active tracking token (ErrTokenInvalid otherwise) — see
	// specs/service-order-tracking/design.md §7.
	FindByCodeAndTokenHash(ctx context.Context, code int64, tokenHash string) (*trackingRead, error)
}

// TrackingService is the "módulo lógico de acompanhamento" from the source
// spec: it owns the token validator (hash the raw token, delegate the
// code/order/revocation checks to the repository).
type TrackingService struct {
	repository TrackingRepository
}

func NewTrackingService(repository TrackingRepository) *TrackingService {
	return &TrackingService{repository: repository}
}

// Get returns the tracking read model for code, if rawToken is the active
// tracking token for that specific order (requirements.md §3.1, §3.2).
func (service *TrackingService) Get(ctx context.Context, code int64, rawToken string) (*trackingRead, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, ErrTokenInvalid
	}
	return service.repository.FindByCodeAndTokenHash(ctx, code, trackingtoken.Hash(rawToken))
}
