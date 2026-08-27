package vehicle

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VehicleRepository is the persistence boundary for the Vehicle aggregate.
// It is defined next to its only consumer (VehicleService) rather than in a
// separate "contracts" package — see specs/vehicle-management/design.md
// §1.1/§1.3.
type VehicleRepository interface {
	Create(ctx context.Context, vehicle *Vehicle) error
	FindByID(ctx context.Context, id uuid.UUID) (*Vehicle, error)
	FindByPlate(ctx context.Context, normalizedPlate string) (*Vehicle, error)
	// ExistsByPlate reports whether normalizedPlate already belongs to a
	// vehicle other than excludeID (nil to check against every vehicle).
	ExistsByPlate(ctx context.Context, normalizedPlate string, excludeID *uuid.UUID) (bool, error)
	List(ctx context.Context, page, pageSize int) ([]*Vehicle, int, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*Vehicle, int, error)
	Update(ctx context.Context, vehicle *Vehicle) error
}

// postgresUniqueViolation is the SQLSTATE Postgres returns for any unique
// index/constraint violation.
const postgresUniqueViolation = "23505"

// licensePlateUniqueConstraint is the name of the unique index declared in
// docs/schema.sql that this repository's writes can violate.
const licensePlateUniqueConstraint = "ux_vehicles_license_plate"

const vehicleColumns = `id, code, license_plate, brand, model, year, color, customer_id, status, created_at, updated_at`

// PostgresVehicleRepository implements VehicleRepository against pgx.
type PostgresVehicleRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresVehicleRepository(pool *pgxpool.Pool) *PostgresVehicleRepository {
	return &PostgresVehicleRepository{pool: pool}
}

func (repository *PostgresVehicleRepository) Create(ctx context.Context, vehicle *Vehicle) error {
	const query = `
		INSERT INTO vehicles (id, license_plate, brand, model, year, color, customer_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING code, created_at, updated_at`

	err := repository.pool.QueryRow(ctx, query,
		vehicle.ID, vehicle.LicensePlate, vehicle.Brand, vehicle.Model, vehicle.Year, vehicle.Color, vehicle.CustomerID, string(vehicle.Status),
	).Scan(&vehicle.Code, &vehicle.CreatedAt, &vehicle.UpdatedAt)

	return mapUniqueViolation(err)
}

func (repository *PostgresVehicleRepository) FindByID(ctx context.Context, id uuid.UUID) (*Vehicle, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+vehicleColumns+` FROM vehicles WHERE id = $1`, id)
	return scanVehicle(row)
}

func (repository *PostgresVehicleRepository) FindByPlate(ctx context.Context, normalizedPlate string) (*Vehicle, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+vehicleColumns+` FROM vehicles WHERE license_plate = $1`, normalizedPlate)
	return scanVehicle(row)
}

func (repository *PostgresVehicleRepository) ExistsByPlate(ctx context.Context, normalizedPlate string, excludeID *uuid.UUID) (bool, error) {
	var exists bool
	var err error

	if excludeID != nil {
		err = repository.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM vehicles WHERE license_plate = $1 AND id <> $2)`,
			normalizedPlate, *excludeID,
		).Scan(&exists)
	} else {
		err = repository.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM vehicles WHERE license_plate = $1)`,
			normalizedPlate,
		).Scan(&exists)
	}

	return exists, err
}

func (repository *PostgresVehicleRepository) List(ctx context.Context, page, pageSize int) ([]*Vehicle, int, error) {
	offset := (page - 1) * pageSize

	rows, err := repository.pool.Query(ctx,
		`SELECT `+vehicleColumns+`, COUNT(*) OVER() AS total
		 FROM vehicles
		 ORDER BY code
		 LIMIT $1 OFFSET $2`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return scanVehicleRows(rows)
}

func (repository *PostgresVehicleRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*Vehicle, int, error) {
	offset := (page - 1) * pageSize

	rows, err := repository.pool.Query(ctx,
		`SELECT `+vehicleColumns+`, COUNT(*) OVER() AS total
		 FROM vehicles
		 WHERE customer_id = $1
		 ORDER BY code
		 LIMIT $2 OFFSET $3`,
		customerID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	return scanVehicleRows(rows)
}

func (repository *PostgresVehicleRepository) Update(ctx context.Context, vehicle *Vehicle) error {
	const query = `
		UPDATE vehicles
		SET brand = $2, model = $3, year = $4, color = $5, status = $6
		WHERE id = $1
		RETURNING updated_at`

	err := repository.pool.QueryRow(ctx, query,
		vehicle.ID, vehicle.Brand, vehicle.Model, vehicle.Year, vehicle.Color, string(vehicle.Status),
	).Scan(&vehicle.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func scanVehicle(row pgx.Row) (*Vehicle, error) {
	vehicle := &Vehicle{}
	var status string

	err := row.Scan(
		&vehicle.ID, &vehicle.Code, &vehicle.LicensePlate, &vehicle.Brand, &vehicle.Model, &vehicle.Year, &vehicle.Color, &vehicle.CustomerID, &status,
		&vehicle.CreatedAt, &vehicle.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	vehicle.Status = Status(status)
	return vehicle, nil
}

func scanVehicleRows(rows pgx.Rows) ([]*Vehicle, int, error) {
	var vehicles []*Vehicle
	total := 0
	for rows.Next() {
		vehicle, status := &Vehicle{}, ""
		if err := rows.Scan(
			&vehicle.ID, &vehicle.Code, &vehicle.LicensePlate, &vehicle.Brand, &vehicle.Model, &vehicle.Year, &vehicle.Color, &vehicle.CustomerID, &status,
			&vehicle.CreatedAt, &vehicle.UpdatedAt, &total,
		); err != nil {
			return nil, 0, err
		}
		vehicle.Status = Status(status)
		vehicles = append(vehicles, vehicle)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return vehicles, total, nil
}

// mapUniqueViolation translates a Postgres unique-constraint-violation error
// into ErrDuplicatePlate, so a race between two concurrent requests is still
// caught even after the application-level pre-check
// (VehicleService.Create's ExistsByPlate call). Any other error — including
// nil — is returned unchanged.
func mapUniqueViolation(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != postgresUniqueViolation {
		return err
	}
	if postgresError.ConstraintName == licensePlateUniqueConstraint {
		return ErrDuplicatePlate
	}
	return err
}
