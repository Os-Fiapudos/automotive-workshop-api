package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCNPJ(t *testing.T) {
	tests := []struct {
		name    string
		cnpj    string
		wantErr bool
	}{
		{"valid CNPJ (legacy numeric)", "11222333000181", false},
		{"wrong length", "1122233300018", true},
		{"wrong first check digit", "11222333000191", true},
		{"wrong second check digit", "11222333000180", true},
		{"all zeros", "00000000000000", true},
		{"all same non-zero digit", "11111111111111", true},

		// Alphanumeric CNPJ (Receita Federal Instrução Normativa RFB nº
		// 2.229/2024, in effect since July 2026): letters A-Z allowed in
		// the first 12 characters, check digits always numeric.
		{"valid alphanumeric CNPJ", "12ABC345000188", false},
		{"alphanumeric with wrong check digits", "12ABC345000199", true},
		{"letter in the check-digit position is always invalid", "12ABC3450001A8", true},
		{"all same letter (non-numeric check digits)", "AAAAAAAAAAAAAA", true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateCNPJ(testCase.cnpj)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCNPJNormalizeAndValidate(t *testing.T) {
	formatted := "11.222.333/0001-81"
	normalized := Normalize(formatted)
	assert.Equal(t, "11222333000181", normalized)
	assert.NoError(t, ValidateCNPJ(normalized))

	document, err := New(formatted)
	assert.NoError(t, err)
	assert.Equal(t, CNPJ, document.Type)
	assert.Equal(t, "11222333000181", document.Value)
}

// TestAlphanumericCNPJNormalizeAndValidate uses the official example from
// Receita Federal's alphanumeric CNPJ documentation, confirming this
// package's independently-implemented check-digit algorithm agrees with it.
func TestAlphanumericCNPJNormalizeAndValidate(t *testing.T) {
	formatted := "12.ABC.345/0001-88"
	normalized := Normalize(formatted)
	assert.Equal(t, "12ABC345000188", normalized)
	assert.NoError(t, ValidateCNPJ(normalized))

	document, err := New(formatted)
	assert.NoError(t, err)
	assert.Equal(t, CNPJ, document.Type)
	assert.Equal(t, "12ABC345000188", document.Value)
}

func TestAlphanumericCNPJNormalizeUppercasesLowercaseLetters(t *testing.T) {
	document, err := New("12.abc.345/0001-88")
	assert.NoError(t, err)
	assert.Equal(t, "12ABC345000188", document.Value)
}
