package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	assert.Equal(t, "12345678909", Normalize("123.456.789-09"))
	assert.Equal(t, "12345678000190", Normalize("12.345.678/0001-90"))
	assert.Equal(t, "", Normalize("---..///"))
	// Letters are kept (needed for the alphanumeric CNPJ, see cnpj.go) and
	// uppercased; punctuation/spaces are still stripped like before.
	assert.Equal(t, "12ABC345000188", Normalize("12.abc.345/0001-88"))
}

func TestDetectType(t *testing.T) {
	documentType, err := DetectType("11144477735")
	assert.NoError(t, err)
	assert.Equal(t, CPF, documentType)

	documentType, err = DetectType("11222333000181")
	assert.NoError(t, err)
	assert.Equal(t, CNPJ, documentType)

	_, err = DetectType("12345")
	assert.Error(t, err)
}

func TestNewRejectsInvalidDocument(t *testing.T) {
	_, err := New("000.000.000-00")
	assert.Error(t, err)

	_, err = New("123")
	assert.Error(t, err)
}
