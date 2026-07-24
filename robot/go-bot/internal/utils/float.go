package utils

import "math"

const epsilon = 1e-9

// IsZeroEps checks if a value is effectively zero within an epsilon margin.
func IsZeroEps(val float64) bool {
	return math.Abs(val) <= epsilon
}

// IsEqualEps checks if two values are effectively equal within an epsilon margin.
func IsEqualEps(a, b float64) bool {
	return math.Abs(a-b) <= epsilon
}
