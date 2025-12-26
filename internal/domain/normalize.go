package domain

import "strings"

// NormalizeHumanName trims leading/trailing whitespace and collapses internal whitespace runs.
// It is used for displayName normalization.
func NormalizeHumanName(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
