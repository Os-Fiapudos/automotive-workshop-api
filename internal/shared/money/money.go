package money

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// Cents is an exact monetary amount in the currency's smallest unit (cents,
// for BRL): Cents(1990) is R$ 19,90.
type Cents int64

// MaxAmount is the largest magnitude a NUMERIC(12,2) column holds
// (9999999999.99). Every value parsed, multiplied, or summed by this package
// is kept within [-MaxAmount, MaxAmount], so a value that passes here never
// overflows the column it is written to.
const MaxAmount Cents = 999_999_999_999

// maxAmountDigits is the number of digits of MaxAmount in cents.
const maxAmountDigits = 12

// ErrInvalidAmount matches (errors.Is) every amount this package rejects:
// ErrTooManyDecimalPlaces and ErrOutOfRange.
var ErrInvalidAmount = errors.New("invalid monetary amount")

var (
	// ErrTooManyDecimalPlaces is returned for a value with more than two
	// decimal places. Such a value is rejected, never rounded or truncated
	// (requirements.md BR7): rounding would silently change what the client
	// sent.
	ErrTooManyDecimalPlaces error = amountError("must have at most two decimal places")

	// ErrOutOfRange is returned when a value, product, or sum exceeds
	// MaxAmount in magnitude.
	ErrOutOfRange error = amountError("must not exceed 9999999999.99")

	// ErrInvalidQuantity is returned by MulQuantity for a quantity that is
	// not strictly positive.
	ErrInvalidQuantity = errors.New("quantity must be greater than zero")
)

// amountError is a rejection reason whose message is safe to return to an API
// client, and which matches ErrInvalidAmount.
type amountError string

func (e amountError) Error() string        { return string(e) }
func (e amountError) Is(target error) bool { return target == ErrInvalidAmount }

// Parse reads a decimal number in JSON number syntax ("19.90", "0.1", "1e2",
// "-5") into Cents exactly. It rejects more than two significant decimal
// places (trailing zeros such as "19.900" are fine) and magnitudes above
// MaxAmount.
func Parse(text string) (Cents, error) {
	negative, intDigits, fracDigits, exponent, ok := splitNumber(text)
	if !ok {
		return 0, ErrInvalidAmount
	}

	// value = digits × 10^-scale
	digits := strings.TrimLeft(intDigits+fracDigits, "0")
	if digits == "" {
		return 0, nil
	}
	scale := int64(len(fracDigits)) - exponent
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = digits[:len(digits)-1]
		scale--
	}
	if scale > 2 {
		return 0, ErrTooManyDecimalPlaces
	}
	// In cents the value has len(digits)+(2-scale) digits.
	if int64(len(digits))+2-scale > maxAmountDigits {
		return 0, ErrOutOfRange
	}
	cents, err := strconv.ParseInt(digits+strings.Repeat("0", int(2-scale)), 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	if negative {
		cents = -cents
	}
	return Cents(cents), nil
}

// splitNumber splits text, in JSON number syntax
// (-?(0|[1-9][0-9]*)(.[0-9]+)?([eE][+-]?[0-9]+)?), into its parts.
func splitNumber(text string) (negative bool, intDigits, fracDigits string, exponent int64, ok bool) {
	rest := text
	if strings.HasPrefix(rest, "-") {
		negative, rest = true, rest[1:]
	}
	end := countDigits(rest)
	if end == 0 || (end > 1 && rest[0] == '0') {
		return false, "", "", 0, false
	}
	intDigits, rest = rest[:end], rest[end:]

	if strings.HasPrefix(rest, ".") {
		end = countDigits(rest[1:])
		if end == 0 {
			return false, "", "", 0, false
		}
		fracDigits, rest = rest[1:1+end], rest[1+end:]
	}

	if strings.HasPrefix(rest, "e") || strings.HasPrefix(rest, "E") {
		exponentText := rest[1:]
		if strings.HasPrefix(exponentText, "+") || strings.HasPrefix(exponentText, "-") {
			exponentText = exponentText[1:]
		}
		// The bound only keeps scale arithmetic far from int64 overflow; any
		// exponent this large is out of range or has too many decimals anyway.
		if end = countDigits(exponentText); end == 0 || end != len(exponentText) || end > 9 {
			return false, "", "", 0, false
		}
		parsed, err := strconv.ParseInt(rest[1:], 10, 64)
		if err != nil {
			return false, "", "", 0, false
		}
		exponent, rest = parsed, ""
	}

	if rest != "" {
		return false, "", "", 0, false
	}
	return negative, intDigits, fracDigits, exponent, true
}

func countDigits(text string) int {
	count := 0
	for count < len(text) && text[count] >= '0' && text[count] <= '9' {
		count++
	}
	return count
}

// MarshalJSON writes c as a JSON number in its shortest exact decimal form —
// 1990 → 19.9, 10000 → 100, 10 → 0.1 — the same text encoding/json produces
// for the float64 these fields used to be, so the wire format is unchanged
// byte for byte (requirements.md BR4).
func (c Cents) MarshalJSON() ([]byte, error) {
	whole, fraction, negative := c.split()
	text := strconv.FormatInt(whole, 10)
	if fraction != 0 {
		text += strings.TrimRight("."+twoDigits(fraction), "0")
	}
	if negative {
		text = "-" + text
	}
	return []byte(text), nil
}

// UnmarshalJSON parses a JSON number via its decimal text (Parse), never
// through float64. A non-number token is reported as a
// *json.UnmarshalTypeError, the same error a float64 field produced; a
// number Parse rejects returns an error matching ErrInvalidAmount.
func (c *Cents) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if string(data) == "null" {
		return nil // same as encoding/json for a non-pointer number: leave as is
	}
	if len(data) == 0 || (data[0] != '-' && (data[0] < '0' || data[0] > '9')) {
		return &json.UnmarshalTypeError{Value: jsonKind(data), Type: reflect.TypeFor[Cents]()}
	}
	parsed, err := Parse(string(data))
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

func jsonKind(data []byte) string {
	switch {
	case len(data) == 0:
		return "empty"
	case data[0] == '"':
		return "string"
	case data[0] == '{':
		return "object"
	case data[0] == '[':
		return "array"
	default:
		return "bool"
	}
}

// String formats c with exactly two decimal places ("19.90", "-0.05"), for
// display and logs.
func (c Cents) String() string {
	whole, fraction, negative := c.split()
	text := strconv.FormatInt(whole, 10) + "." + twoDigits(fraction)
	if negative {
		text = "-" + text
	}
	return text
}

// split returns c's magnitude as whole units and cents. Go's division
// truncates toward zero, so for a negative c both parts are non-positive and
// negating them cannot overflow — not even for math.MinInt64, unlike
// negating c itself.
func (c Cents) split() (whole, fraction int64, negative bool) {
	whole, fraction = int64(c/100), int64(c%100)
	if c < 0 {
		whole, fraction = -whole, -fraction
	}
	return whole, fraction, c < 0
}

func twoDigits(value int64) string {
	if value < 10 {
		return "0" + strconv.FormatInt(value, 10)
	}
	return strconv.FormatInt(value, 10)
}

// Validate reports ErrOutOfRange when c's magnitude exceeds MaxAmount.
func (c Cents) Validate() error {
	if c > MaxAmount || c < -MaxAmount {
		return ErrOutOfRange
	}
	return nil
}

// MulQuantity returns c × quantity — a unit price times an item quantity.
// quantity must be strictly positive (ErrInvalidQuantity), and both c and
// the product must stay within MaxAmount (ErrOutOfRange); the bound is
// checked before multiplying, so int64 never overflows.
func (c Cents) MulQuantity(quantity int) (Cents, error) {
	if quantity <= 0 {
		return 0, ErrInvalidQuantity
	}
	if err := c.Validate(); err != nil {
		return 0, err
	}
	magnitude := c
	if magnitude < 0 {
		magnitude = -magnitude
	}
	if magnitude != 0 && int64(quantity) > int64(MaxAmount/magnitude) {
		return 0, ErrOutOfRange
	}
	return c * Cents(quantity), nil
}

// Add returns c + other, rejecting (ErrOutOfRange) an operand or a result
// beyond MaxAmount. Both operands are bounded first, so the int64 sum cannot
// overflow.
func (c Cents) Add(other Cents) (Cents, error) {
	if err := c.Validate(); err != nil {
		return 0, err
	}
	if err := other.Validate(); err != nil {
		return 0, err
	}
	sum := c + other
	if err := sum.Validate(); err != nil {
		return 0, err
	}
	return sum, nil
}

// Sum adds values with Add's bounds; an empty list sums to zero.
func Sum(values ...Cents) (Cents, error) {
	var total Cents
	for _, value := range values {
		var err error
		if total, err = total.Add(value); err != nil {
			return 0, err
		}
	}
	return total, nil
}
