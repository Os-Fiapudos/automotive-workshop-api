package servicetracking

import (
	"context"
	"errors"
	"testing"
	"time"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// fakeRepository is an in-memory TrackingRepository used only by these
// service-level unit tests — no mocking framework, same convention as
// internal/features/service-order/fake_repository_test.go.
type fakeRepository struct {
	// orders maps a service order's code to (its stored token hash, whether
	// that token is revoked, the read model to return on a match).
	orders map[int64]fakeOrder
}

type fakeOrder struct {
	tokenHash string
	revoked   bool
	read      *trackingRead
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{orders: make(map[int64]fakeOrder)}
}

func (fake *fakeRepository) seed(code int64, rawToken string, revoked bool) {
	fake.orders[code] = fakeOrder{
		tokenHash: trackingtoken.Hash(rawToken),
		revoked:   revoked,
		read: &trackingRead{
			Code:   code,
			Status: "RECEIVED",
			Vehicle: trackingVehicle{
				LicensePlate: "ABC1D23",
				Brand:        "Fiat",
				Model:        "Uno",
				Year:         2020,
				Color:        "White",
			},
			Milestones: []trackingMilestone{
				{Event: "creation", PreviousStatus: "RECEIVED", NewStatus: "RECEIVED", OccurredAt: time.Now()},
			},
		},
	}
}

func (fake *fakeRepository) FindByCodeAndTokenHash(_ context.Context, code int64, tokenHash string) (*trackingRead, error) {
	order, ok := fake.orders[code]
	if !ok {
		return nil, ErrOrderNotFound
	}
	if order.revoked || order.tokenHash != tokenHash {
		return nil, ErrTokenInvalid
	}
	return order.read, nil
}

func TestGetValidToken(t *testing.T) {
	repo := newFakeRepository()
	repo.seed(1042, "valid-raw-token", false)
	service := NewTrackingService(repo)

	read, err := service.Get(context.Background(), 1042, "valid-raw-token")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if read.Code != 1042 {
		t.Fatalf("expected code 1042, got %d", read.Code)
	}
	if read.Vehicle.LicensePlate != "ABC1D23" {
		t.Fatalf("expected the seeded vehicle plate, got %q", read.Vehicle.LicensePlate)
	}
	if len(read.Milestones) != 1 {
		t.Fatalf("expected 1 milestone, got %d", len(read.Milestones))
	}
}

func TestGetUnknownCode(t *testing.T) {
	repo := newFakeRepository()
	service := NewTrackingService(repo)

	_, err := service.Get(context.Background(), 9999, "any-token")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestGetMissingToken(t *testing.T) {
	repo := newFakeRepository()
	repo.seed(1042, "valid-raw-token", false)
	service := NewTrackingService(repo)

	_, err := service.Get(context.Background(), 1042, "")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestGetWrongToken(t *testing.T) {
	repo := newFakeRepository()
	repo.seed(1042, "valid-raw-token", false)
	service := NewTrackingService(repo)

	_, err := service.Get(context.Background(), 1042, "wrong-token")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestGetRevokedToken(t *testing.T) {
	repo := newFakeRepository()
	repo.seed(1042, "valid-raw-token", true)
	service := NewTrackingService(repo)

	_, err := service.Get(context.Background(), 1042, "valid-raw-token")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestGetCrossOrderToken(t *testing.T) {
	repo := newFakeRepository()
	repo.seed(1042, "token-for-order-a", false)
	repo.seed(2001, "token-for-order-b", false)
	service := NewTrackingService(repo)

	// Order B's token must not grant access to order A.
	_, err := service.Get(context.Background(), 1042, "token-for-order-b")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected ErrTokenInvalid for a cross-order token, got %v", err)
	}
}
