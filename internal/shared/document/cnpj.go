package document

import "fmt"

var (
	cnpjFirstDigitWeights  = []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	cnpjSecondDigitWeights = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
)

// ValidateCNPJ checks that normalized has 14 characters and that its two
// check digits match the official CNPJ modulo-11 algorithm.
//
// Since July 2026 (Receita Federal's Instrução Normativa RFB nº 2.229/2024),
// a CNPJ's first 12 characters may be digits or uppercase letters A-Z — the
// "alphanumeric CNPJ" — while its last 2 characters (the check digits)
// remain numeric, always. Pre-existing numeric CNPJs are not reissued and
// stay valid, so this function accepts both the legacy all-numeric form and
// the new alphanumeric form; it does not require letters to be present.
//
// It also rejects the degenerate all-same-character sequence, which
// satisfies the check-digit math but is never a real CNPJ.
func ValidateCNPJ(normalized string) error {
	if len(normalized) != 14 {
		return fmt.Errorf("CNPJ must have 14 characters, got %d", len(normalized))
	}
	if !isAllDigits(normalized[12:]) {
		return fmt.Errorf("CNPJ %q must have numeric check digits", normalized)
	}
	if allSameCharacter(normalized) {
		return fmt.Errorf("CNPJ %q is not valid: all characters are the same", normalized)
	}

	values, err := toCharacterValues(normalized)
	if err != nil {
		return fmt.Errorf("CNPJ %q: %w", normalized, err)
	}

	firstCheckDigit := checkDigit(values[:12], cnpjFirstDigitWeights)
	baseWithFirstCheckDigit := append(append([]int{}, values[:12]...), firstCheckDigit)
	secondCheckDigit := checkDigit(baseWithFirstCheckDigit, cnpjSecondDigitWeights)

	if values[12] != firstCheckDigit || values[13] != secondCheckDigit {
		return fmt.Errorf("CNPJ %q has invalid check digits", normalized)
	}
	return nil
}
