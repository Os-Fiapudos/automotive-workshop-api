package vehicle

import (
	"fmt"
	"regexp"
	"strings"
)

// platePattern matches a normalized (7-character, uppercase) Brazilian
// license plate in either circulating format: legacy (AAA9999) or Mercosul
// (AAA9A99, mandatory since 2018 — legacy plates remain valid until
// replaced). Position 5 is the only difference between the two formats — a
// digit for legacy, a letter for Mercosul — so [A-Z0-9] there accepts both
// without needing two separate patterns or a format-detection step.
var platePattern = regexp.MustCompile(`^[A-Z]{3}[0-9][A-Z0-9][0-9]{2}$`)

// NormalizePlate strips formatting characters (spaces, hyphens) and
// uppercases any letters it keeps, mirroring
// internal/shared/document.Normalize's shape for the analogous CPF/CNPJ
// case.
func NormalizePlate(raw string) string {
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

// ValidatePlate reports whether normalized is a structurally valid Brazilian
// license plate (legacy or Mercosul format). There is no official
// check-digit algorithm for plates, unlike CPF/CNPJ — validation is
// structural only.
func ValidatePlate(normalized string) error {
	if !platePattern.MatchString(normalized) {
		return fmt.Errorf("license plate must match the legacy (AAA9999) or Mercosul (AAA9A99) format")
	}
	return nil
}

// NewPlate normalizes raw and validates its structure, returning the
// normalized plate only when raw represents a structurally valid license
// plate. It never accepts a plate on length alone.
func NewPlate(raw string) (string, error) {
	normalized := NormalizePlate(raw)
	if err := ValidatePlate(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}
