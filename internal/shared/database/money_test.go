package database

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/shared/money"
)

func TestNumericRoundTrip(t *testing.T) {
	for _, cents := range []money.Cents{0, 10, 1990, 123456789, money.MaxAmount, -525} {
		got, err := CentsFromNumeric(NumericFromCents(cents))
		require.NoError(t, err, cents)
		assert.Equal(t, cents, got)
	}
}

func TestCentsFromNumericNormalizesExponent(t *testing.T) {
	cases := []struct {
		n    pgtype.Numeric
		want money.Cents
	}{
		{pgtype.Numeric{Int: big.NewInt(199), Exp: -1, Valid: true}, 1990},   // 19.9
		{pgtype.Numeric{Int: big.NewInt(19900), Exp: -3, Valid: true}, 1990}, // 19.900
		{pgtype.Numeric{Int: big.NewInt(1), Exp: 2, Valid: true}, 10000},     // 1e2
		{pgtype.Numeric{Int: big.NewInt(0), Exp: -2, Valid: true}, 0},        // 0.00
		{pgtype.Numeric{Int: big.NewInt(-5), Exp: -2, Valid: true}, -5},      // -0.05
		{pgtype.Numeric{Int: big.NewInt(1), Exp: 0, Valid: true}, 100},       // 1
		{pgtype.Numeric{Int: big.NewInt(999999999999), Exp: -2, Valid: true}, money.MaxAmount},
	}
	for _, tc := range cases {
		got, err := CentsFromNumeric(tc.n)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestCentsFromNumericRejects(t *testing.T) {
	huge, _ := new(big.Int).SetString("100000000000000000000000", 10)
	cases := map[string]pgtype.Numeric{
		"null":          {},
		"nan":           {NaN: true, Valid: true},
		"infinity":      {InfinityModifier: pgtype.Infinity, Valid: true},
		"three places":  {Int: big.NewInt(19999), Exp: -3, Valid: true},
		"above max":     {Int: big.NewInt(1_000_000_000_000), Exp: -2, Valid: true},
		"beyond int64":  {Int: huge, Exp: 0, Valid: true},
		"nil mantissa":  {Valid: true},
		"tiny fraction": {Int: big.NewInt(1), Exp: -20, Valid: true},
	}
	for name, n := range cases {
		_, err := CentsFromNumeric(n)
		assert.Error(t, err, name)
	}
}

func TestScanCents(t *testing.T) {
	var value money.Cents
	require.NoError(t, ScanCents(&value).ScanNumeric(NumericFromCents(1990)))
	assert.Equal(t, money.Cents(1990), value)

	assert.Error(t, ScanCents(&value).ScanNumeric(pgtype.Numeric{}))
	assert.Equal(t, money.Cents(1990), value, "a failed scan leaves dst untouched")
}
