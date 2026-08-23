package servicetracking

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTrackingRepository implements TrackingRepository against pgx,
// reading service_orders/vehicles/service_order_history/
// service_order_tracking_tokens directly via parameterized SQL — the same
// established pattern internal/features/service-order already uses to read
// data belonging to another feature's table (customerRef/vehicleRef in
// service-order/repository.go), rather than importing that feature's Go
// package (specs/service-order-tracking/design.md, header).
type PostgresTrackingRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresTrackingRepository(pool *pgxpool.Pool) *PostgresTrackingRepository {
	return &PostgresTrackingRepository{pool: pool}
}

// FindByCodeAndTokenHash implements TrackingRepository (design.md §7): the
// order is resolved by code first, so an unknown code always maps to
// ErrOrderNotFound regardless of the token; only then is tokenHash checked
// against that specific order's active token, so a wrong/foreign/revoked
// token always maps to ErrTokenInvalid without ever needing three separate
// error cases at the handler level (requirements.md AC3-AC5).
func (repository *PostgresTrackingRepository) FindByCodeAndTokenHash(ctx context.Context, code int64, tokenHash string) (*trackingRead, error) {
	var orderID, vehicleID uuid.UUID
	var status string
	if err := repository.pool.QueryRow(ctx,
		`SELECT id, vehicle_id, status FROM service_orders WHERE code = $1`, code,
	).Scan(&orderID, &vehicleID, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	var matches bool
	if err := repository.pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM service_order_tracking_tokens
			WHERE service_order_id = $1 AND token_hash = $2 AND revoked_at IS NULL
		)`, orderID, tokenHash,
	).Scan(&matches); err != nil {
		return nil, err
	}
	if !matches {
		return nil, ErrTokenInvalid
	}

	vehicle, err := repository.findVehicle(ctx, vehicleID)
	if err != nil {
		return nil, err
	}

	milestones, err := repository.findMilestones(ctx, orderID)
	if err != nil {
		return nil, err
	}

	return &trackingRead{
		Code:       code,
		Status:     status,
		Vehicle:    *vehicle,
		Milestones: milestones,
	}, nil
}

func (repository *PostgresTrackingRepository) findVehicle(ctx context.Context, vehicleID uuid.UUID) (*trackingVehicle, error) {
	vehicle := &trackingVehicle{}
	if err := repository.pool.QueryRow(ctx,
		`SELECT license_plate, brand, model, year, color FROM vehicles WHERE id = $1`, vehicleID,
	).Scan(&vehicle.LicensePlate, &vehicle.Brand, &vehicle.Model, &vehicle.Year, &vehicle.Color); err != nil {
		return nil, err
	}
	return vehicle, nil
}

func (repository *PostgresTrackingRepository) findMilestones(ctx context.Context, orderID uuid.UUID) ([]trackingMilestone, error) {
	rows, err := repository.pool.Query(ctx,
		`SELECT event, previous_status, new_status, occurred_at
		 FROM service_order_history
		 WHERE service_order_id = $1
		 ORDER BY occurred_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	milestones := make([]trackingMilestone, 0)
	for rows.Next() {
		var milestone trackingMilestone
		if err := rows.Scan(&milestone.Event, &milestone.PreviousStatus, &milestone.NewStatus, &milestone.OccurredAt); err != nil {
			return nil, err
		}
		milestones = append(milestones, milestone)
	}
	return milestones, rows.Err()
}
