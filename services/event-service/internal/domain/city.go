package domain

import (
	"strings"
)

// NormalizeCity converts city input to normalized form for storage and querying.
// Examples: "Sydney" -> "sydney", "  MELBOURNE  " -> "melbourne"
func NormalizeCity(input string) string {
	// Trim whitespace
	normalized := strings.TrimSpace(input)

	// Convert to lowercase
	normalized = strings.ToLower(normalized)

	return normalized
}
