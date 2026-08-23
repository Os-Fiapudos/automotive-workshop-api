package vehicle

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Status is the vehicle's situation: ACTIVE or INACTIVE.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// minYear is the earliest manufacturing year accepted (requirements.md BR5)
// — excludes implausible/mistyped years while still allowing genuinely old
// vehicles to be registered.
const minYear = 1950

// Vehicle is the domain aggregate for this feature. A Vehicle cannot exist
// with a structurally invalid or unnormalized license plate, or a
// manufacturing year outside the valid range — both are enforced by
// NewVehicle/UpdateDetails, never by the HTTP layer alone.
type Vehicle struct {
	ID           uuid.UUID
	Code         int64
	LicensePlate string
	Brand        string
	Model        string
	Year         int
	Color        string
	CustomerID   uuid.UUID
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewVehicle builds a new vehicle that always starts ACTIVE (see
// specs/vehicle-management/requirements.md BR6). rawLicensePlate is
// normalized and structurally validated internally; Code/CreatedAt/UpdatedAt
// are assigned by the database on insert.
func NewVehicle(rawLicensePlate, brand, model string, year int, color string, customerID uuid.UUID) (*Vehicle, error) {
	normalizedPlate, err := NewPlate(rawLicensePlate)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPlate, err)
	}

	if err := validateYear(year); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidYear, err)
	}

	return &Vehicle{
		ID:           uuid.New(),
		LicensePlate: normalizedPlate,
		Brand:        brand,
		Model:        model,
		Year:         year,
		Color:        color,
		CustomerID:   customerID,
		Status:       StatusActive,
	}, nil
}

// UpdateDetails replaces brand, model, year, and color in one call, after
// re-validating year. License plate and customer id are never touched here —
// this feature has no path that mutates either after creation
// (requirements.md §3.2).
func (vehicle *Vehicle) UpdateDetails(brand, model string, year int, color string) error {
	if err := validateYear(year); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidYear, err)
	}

	vehicle.Brand = brand
	vehicle.Model = model
	vehicle.Year = year
	vehicle.Color = color
	return nil
}

// Deactivate moves the vehicle to INACTIVE. It is idempotent: deactivating
// an already-inactive vehicle is a no-op, not an error. There is
// deliberately no Activate method (requirements.md BR7).
func (vehicle *Vehicle) Deactivate() {
	vehicle.Status = StatusInactive
}

// IsActive reports whether the vehicle is currently ACTIVE.
func (vehicle *Vehicle) IsActive() bool {
	return vehicle.Status == StatusActive
}

// validateYear reports whether year falls within the valid manufacturing
// range: 1950 to the current year + 1 inclusive (requirements.md BR5) — the
// "+1" allows next-year models sold in advance.
func validateYear(year int) error {
	maxYear := time.Now().Year() + 1
	if year < minYear || year > maxYear {
		return fmt.Errorf("year must be between %d and %d", minYear, maxYear)
	}
	return nil
}
