package money

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	cases := map[string]Cents{
		"0":             0,
		"-0":            0,
		"0.1":           10,
		"0.01":          1,
		"19.90":         1990,
		"19.9":          1990,
		"19.900":        1990, // trailing zeros are not extra precision
		"1234567.89":    123456789,
		"100":           10000,
		"1e2":           10000,
		"1.5E+1":        1500,
		"1990e-2":       1990,
		"-5.25":         -525,
		"9999999999.99": MaxAmount,
		"0.00e5":        0,
	}
	for text, want := range cases {
		got, err := Parse(text)
		require.NoError(t, err, text)
		assert.Equal(t, want, got, text)
	}
}

func TestParseRejectsMoreThanTwoDecimalPlaces(t *testing.T) {
	for _, text := range []string{"19.999", "0.001", "1e-3", "10.005"} {
		_, err := Parse(text)
		assert.ErrorIs(t, err, ErrTooManyDecimalPlaces, text)
		assert.ErrorIs(t, err, ErrInvalidAmount, text)
	}
}

func TestParseRejectsOutOfRange(t *testing.T) {
	for _, text := range []string{"10000000000", "10000000000.00", "-10000000000", "1e10", "99999999999999999999999", "1e999999999"} {
		_, err := Parse(text)
		assert.ErrorIs(t, err, ErrOutOfRange, text)
		assert.ErrorIs(t, err, ErrInvalidAmount, text)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	for _, text := range []string{"", "-", "abc", "1.", ".5", "+5", "01", "1e", "1e+", "1.2.3", "1,5", "NaN", "1e1234567890"} {
		_, err := Parse(text)
		assert.ErrorIs(t, err, ErrInvalidAmount, text)
	}
}

// TestJSONRoundTrip covers values a naive float64 path is known to mangle
// (0.1, 19.90, 1234567.89 — requirements.md AC2) plus 0 and the
// NUMERIC(12,2) limit.
func TestJSONRoundTrip(t *testing.T) {
	cases := map[string]Cents{
		"0":             0,
		"0.1":           10,
		"0.3":           30,
		"19.9":          1990,
		"1234567.89":    123456789,
		"100":           10000,
		"9999999999.99": MaxAmount,
		"-0.05":         -5,
	}
	for text, cents := range cases {
		var decoded Cents
		require.NoError(t, json.Unmarshal([]byte(text), &decoded), text)
		assert.Equal(t, cents, decoded, text)

		encoded, err := json.Marshal(decoded)
		require.NoError(t, err)
		assert.Equal(t, text, string(encoded))
	}
}

// TestMarshalJSONMatchesFloat64Encoding proves the wire format is unchanged:
// for every two-decimal value up to the column limit, Cents encodes to the
// exact bytes encoding/json produced for the old float64 field (BR4, AC7).
func TestMarshalJSONMatchesFloat64Encoding(t *testing.T) {
	values := []Cents{0, 1, 5, 10, 99, 100, 1990, 2050, 3590, 15180, 45050, 123456789, 100000000000, MaxAmount, -1990}
	for _, cents := range []Cents{0, 1, 999, 1001, 99999} {
		values = append(values, cents*7919)
	}
	for _, cents := range values {
		got, err := json.Marshal(cents)
		require.NoError(t, err)
		want, err := json.Marshal(float64(cents) / 100)
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "cents=%d", cents)
	}
}

func TestUnmarshalJSONInStruct(t *testing.T) {
	var body struct {
		Price    *Cents `json:"price"`
		Optional *Cents `json:"optional"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"price": 19.90, "optional": null}`), &body))
	require.NotNil(t, body.Price)
	assert.Equal(t, Cents(1990), *body.Price)
	assert.Nil(t, body.Optional)
}

func TestUnmarshalJSONRejectsExtraPrecision(t *testing.T) {
	var body struct {
		Price Cents `json:"price"`
	}
	err := json.Unmarshal([]byte(`{"price": 19.999}`), &body)
	assert.ErrorIs(t, err, ErrTooManyDecimalPlaces)
	assert.ErrorIs(t, err, ErrInvalidAmount)
}

func TestUnmarshalJSONRejectsNonNumbersAsTypeError(t *testing.T) {
	for _, raw := range []string{`"19.90"`, `true`, `{}`, `[]`} {
		var value Cents
		err := json.Unmarshal([]byte(raw), &value)
		var typeError *json.UnmarshalTypeError
		assert.True(t, errors.As(err, &typeError), raw)
		assert.NotErrorIs(t, err, ErrInvalidAmount, raw)
	}
}

func TestUnmarshalJSONNullLeavesValue(t *testing.T) {
	value := Cents(42)
	require.NoError(t, value.UnmarshalJSON([]byte("null")))
	assert.Equal(t, Cents(42), value)
}

func TestString(t *testing.T) {
	assert.Equal(t, "0.00", Cents(0).String())
	assert.Equal(t, "0.10", Cents(10).String())
	assert.Equal(t, "19.90", Cents(1990).String())
	assert.Equal(t, "-0.05", Cents(-5).String())
	assert.Equal(t, "9999999999.99", MaxAmount.String())
	assert.Equal(t, "-92233720368547758.08", Cents(math.MinInt64).String())
}

func TestMulQuantity(t *testing.T) {
	total, err := Cents(1990).MulQuantity(3)
	require.NoError(t, err)
	assert.Equal(t, Cents(5970), total)

	total, err = Cents(0).MulQuantity(1_000_000)
	require.NoError(t, err)
	assert.Equal(t, Cents(0), total)

	total, err = Cents(1).MulQuantity(int(MaxAmount))
	require.NoError(t, err)
	assert.Equal(t, MaxAmount, total)

	total, err = MaxAmount.MulQuantity(1)
	require.NoError(t, err)
	assert.Equal(t, MaxAmount, total)
}

func TestMulQuantityRejectsInvalidQuantity(t *testing.T) {
	for _, quantity := range []int{0, -1} {
		_, err := Cents(1990).MulQuantity(quantity)
		assert.ErrorIs(t, err, ErrInvalidQuantity)
	}
}

func TestMulQuantityRejectsOutOfRange(t *testing.T) {
	_, err := MaxAmount.MulQuantity(2)
	assert.ErrorIs(t, err, ErrOutOfRange)

	_, err = Cents(1).MulQuantity(int(MaxAmount) + 1)
	assert.ErrorIs(t, err, ErrOutOfRange)

	// Would overflow int64 if multiplied unchecked.
	_, err = Cents(500_000_000_000).MulQuantity(math.MaxInt32)
	assert.ErrorIs(t, err, ErrOutOfRange)

	_, err = Cents(math.MaxInt64).MulQuantity(1)
	assert.ErrorIs(t, err, ErrOutOfRange)
}

func TestAddAndSum(t *testing.T) {
	// 0.1 + 0.2 == 0.3 exactly, unlike float64.
	total, err := Cents(10).Add(20)
	require.NoError(t, err)
	assert.Equal(t, Cents(30), total)

	total, err = Sum()
	require.NoError(t, err)
	assert.Equal(t, Cents(0), total)

	total, err = Sum(MaxAmount-1, 1)
	require.NoError(t, err)
	assert.Equal(t, MaxAmount, total)

	_, err = Sum(MaxAmount, 1)
	assert.ErrorIs(t, err, ErrOutOfRange)

	_, err = Cents(math.MaxInt64).Add(1)
	assert.ErrorIs(t, err, ErrOutOfRange)
}
