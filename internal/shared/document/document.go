package document

import (
	"fmt"
	"strings"
)

// Type identifies the kind of Brazilian tax document.
type Type string

const (
	CPF  Type = "CPF"
	CNPJ Type = "CNPJ"
)

// Document is a normalized and check-digit-validated Brazilian tax document.
type Document struct {
	Value string
	Type  Type
}

// Normalize strips formatting characters (spaces, punctuation) and
// uppercases any letters it keeps, in addition to keeping digits. CPF is
// always purely numeric; CNPJ may contain uppercase letters A-Z in its first
// 12 characters since Receita Federal's alphanumeric CNPJ format took effect
// in July 2026 (Instrução Normativa RFB nº 2.229/2024) — its last two
// characters (the check digits) remain numeric. This single function safely
// normalizes either document type without needing to know in advance which
// one raw is: a stray letter in what's meant to be a CPF simply fails
// ValidateCPF's digits-only check afterward.
func Normalize(raw string) string {
	var builder strings.Builder
	for _, char := range raw {
		switch {
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char - 32) // uppercase
		}
	}
	return builder.String()
}

// DetectType returns the document type implied by the length of a normalized
// document: 11 characters for CPF, 14 for CNPJ. normalized must already be
// produced by Normalize.
func DetectType(normalized string) (Type, error) {
	switch len(normalized) {
	case 11:
		return CPF, nil
	case 14:
		return CNPJ, nil
	default:
		return "", fmt.Errorf("document must have 11 (CPF) or 14 (CNPJ) characters, got %d", len(normalized))
	}
}

// New normalizes raw, detects its type from its length, and validates its
// check digits, returning a Document only when raw represents a structurally
// valid CPF or CNPJ. It never accepts a document on length/regex alone.
func New(raw string) (Document, error) {
	normalized := Normalize(raw)
	documentType, err := DetectType(normalized)
	if err != nil {
		return Document{}, err
	}

	switch documentType {
	case CPF:
		if err := ValidateCPF(normalized); err != nil {
			return Document{}, err
		}
	case CNPJ:
		if err := ValidateCNPJ(normalized); err != nil {
			return Document{}, err
		}
	}

	return Document{Value: normalized, Type: documentType}, nil
}

// isAllDigits reports whether every character in value is a digit.
func isAllDigits(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

// allSameCharacter reports whether every character in normalized is
// identical (e.g. "00000000000" or "AAAAAAAAAAAAAA") — a degenerate value
// that can satisfy the check-digit math trivially but is never a real
// document.
func allSameCharacter(normalized string) bool {
	for index := 1; index < len(normalized); index++ {
		if normalized[index] != normalized[0] {
			return false
		}
	}
	return true
}

// characterValue converts a single digit or uppercase-letter character to
// its numeric value for check-digit calculations: digits keep their value
// (0-9); letters A-Z map to 17-42. This is "ASCII code minus 48," the
// conversion rule Receita Federal defined for the alphanumeric CNPJ — not
// something invented here.
func characterValue(char byte) (int, error) {
	switch {
	case char >= '0' && char <= '9':
		return int(char - '0'), nil
	case char >= 'A' && char <= 'Z':
		return int(char) - 48, nil
	default:
		return 0, fmt.Errorf("invalid character %q in document", char)
	}
}

// toCharacterValues converts every character of normalized via
// characterValue, in order.
func toCharacterValues(normalized string) ([]int, error) {
	values := make([]int, len(normalized))
	for index := 0; index < len(normalized); index++ {
		value, err := characterValue(normalized[index])
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return values, nil
}

// checkDigit implements the shared CPF/CNPJ modulo-11 check-digit rule: sum
// each character's converted value weighted by a descending sequence, then
// map the remainder of that sum modulo 11 to a single digit (0 when the
// remainder is 0 or 1).
func checkDigit(values []int, weights []int) int {
	sum := 0
	for index, value := range values {
		sum += value * weights[index]
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}
