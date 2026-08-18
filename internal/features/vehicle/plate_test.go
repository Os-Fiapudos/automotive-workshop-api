package vehicle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePlate(t *testing.T) {
	cases := map[string]string{
		"ABC1D23":  "ABC1D23",
		"abc1d23":  "ABC1D23",
		"ABC-1234": "ABC1234",
		"abc 1234": "ABC1234",
		" ABC1234": "ABC1234",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, NormalizePlate(input), "input=%q", input)
	}
}

func TestValidatePlateAcceptsLegacyAndMercosul(t *testing.T) {
	valid := []string{
		"ABC1234", // legacy
		"ABC1D23", // Mercosul
		"DEF4E56", // Mercosul, matches docs/seed.sql
		"GHI9999", // legacy
	}
	for _, plate := range valid {
		assert.NoError(t, ValidatePlate(plate), "plate=%q", plate)
	}
}

func TestValidatePlateRejectsInvalidFormats(t *testing.T) {
	invalid := []string{
		"",
		"ABC123",   // too short
		"ABC12345", // too long
		"AB1C234",  // letters not in the first 3 positions
		"1234ABC",  // wrong shape entirely
		"ABCDE23",  // two letters where a digit is required (position 4)
		"ABC1D2E",  // letter where the last two must be digits
		"aaaaaaa",  // normalize first — raw, un-normalized input
	}
	for _, plate := range invalid {
		assert.Error(t, ValidatePlate(NormalizePlate(plate)), "plate=%q", plate)
	}
}

func TestNewPlateNormalizesThenValidates(t *testing.T) {
	normalized, err := NewPlate("abc-1d23")
	require.NoError(t, err)
	assert.Equal(t, "ABC1D23", normalized)

	_, err = NewPlate("not a plate")
	require.Error(t, err)
}
