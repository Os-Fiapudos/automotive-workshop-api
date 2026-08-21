// Package document normalizes, classifies, and validates Brazilian tax
// documents (CPF and CNPJ). It is deliberately generic and feature-agnostic:
// any feature that needs to identify a person or company by CPF/CNPJ reuses
// it instead of re-implementing the check-digit algorithm.
package document
