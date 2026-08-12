package document

import "fmt"

var (
	cpfFirstDigitWeights  = []int{10, 9, 8, 7, 6, 5, 4, 3, 2}
	cpfSecondDigitWeights = []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2}
)

// ValidateCPF checks that normalized has 11 digits — CPF is always purely
// numeric, unaffected by the alphanumeric CNPJ change (see ValidateCNPJ) —
// and that its two check digits match the official CPF modulo-11 algorithm.
// It also rejects the well-known all-repeated-digit sequences (e.g.
// "00000000000", "11111111111"), which satisfy the check-digit math but are
// not valid CPFs.
func ValidateCPF(normalized string) error {
	if len(normalized) != 11 {
		return fmt.Errorf("CPF must have 11 digits, got %d", len(normalized))
	}
	if !isAllDigits(normalized) {
		return fmt.Errorf("CPF %q must contain only digits", normalized)
	}
	if allSameCharacter(normalized) {
		return fmt.Errorf("CPF %q is not valid: all digits are the same", normalized)
	}

	values, err := toCharacterValues(normalized)
	if err != nil {
		return err
	}

	firstCheckDigit := checkDigit(values[:9], cpfFirstDigitWeights)
	baseWithFirstCheckDigit := append(append([]int{}, values[:9]...), firstCheckDigit)
	secondCheckDigit := checkDigit(baseWithFirstCheckDigit, cpfSecondDigitWeights)

	if values[9] != firstCheckDigit || values[10] != secondCheckDigit {
		return fmt.Errorf("CPF %q has invalid check digits", normalized)
	}
	return nil
}
