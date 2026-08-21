package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCPF(t *testing.T) {
	tests := []struct {
		name    string
		cpf     string
		wantErr bool
	}{
		{"valid CPF", "11144477735", false},
		{"another valid CPF", "52998224725", false},
		{"wrong length", "1114447773", true},
		{"wrong first check digit", "11144477745", true},
		{"wrong second check digit", "11144477736", true},
		{"all zeros", "00000000000", true},
		{"all same non-zero digit", "11111111111", true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateCPF(testCase.cpf)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCPFNormalizeAndValidate(t *testing.T) {
	formatted := "111.444.777-35"
	normalized := Normalize(formatted)
	assert.Equal(t, "11144477735", normalized)
	assert.NoError(t, ValidateCPF(normalized))

	document, err := New(formatted)
	assert.NoError(t, err)
	assert.Equal(t, CPF, document.Type)
	assert.Equal(t, "11144477735", document.Value)
}
