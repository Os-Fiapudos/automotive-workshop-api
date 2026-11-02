package database

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"

	"automotive-workshop-api/internal/shared/money"
)

// NumericFromCents converts c into the pgtype.Numeric bound for a
// NUMERIC(12,2) column: c × 10^-2, built from integers only.
func NumericFromCents(c money.Cents) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(c)), Exp: -2, Valid: true}
}

// CentsFromNumeric converts a NUMERIC read from Postgres into money.Cents
// exactly, working on the numeric's integer mantissa and base-10 exponent —
// no float64 in between. It fails on NULL, NaN/infinity, a value with more
// than two significant decimal places, or a magnitude beyond money.MaxAmount
// (none of which a NOT NULL NUMERIC(12,2) column can hold).
func CentsFromNumeric(n pgtype.Numeric) (money.Cents, error) {
	if !n.Valid {
		return 0, errors.New("database: NULL where a monetary NUMERIC was expected")
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return 0, errors.New("database: monetary NUMERIC is not a finite number")
	}

	// value = Int × 10^Exp, so cents = Int × 10^(Exp+2).
	cents := new(big.Int).Set(n.Int)
	if shift := int64(n.Exp) + 2; shift >= 0 {
		cents.Mul(cents, new(big.Int).Exp(big.NewInt(10), big.NewInt(shift), nil))
	} else {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(-shift), nil)
		var remainder big.Int
		cents.QuoRem(cents, divisor, &remainder)
		if remainder.Sign() != 0 {
			return 0, fmt.Errorf("database: monetary NUMERIC: %w", money.ErrTooManyDecimalPlaces)
		}
	}

	if !cents.IsInt64() {
		return 0, fmt.Errorf("database: monetary NUMERIC: %w", money.ErrOutOfRange)
	}
	value := money.Cents(cents.Int64())
	if err := value.Validate(); err != nil {
		return 0, fmt.Errorf("database: monetary NUMERIC: %w", err)
	}
	return value, nil
}

// ScanCents returns a scan target that reads a NUMERIC column into dst
// through CentsFromNumeric, for use directly in Row.Scan:
//
//	row.Scan(&s.ID, database.ScanCents(&s.Price))
func ScanCents(dst *money.Cents) pgtype.NumericScanner {
	return centsScanner{dst: dst}
}

type centsScanner struct {
	dst *money.Cents
}

func (s centsScanner) ScanNumeric(n pgtype.Numeric) error {
	value, err := CentsFromNumeric(n)
	if err != nil {
		return err
	}
	*s.dst = value
	return nil
}
