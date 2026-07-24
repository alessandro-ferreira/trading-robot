//go:build unit

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsZeroEps(t *testing.T) {
	tests := []struct {
		name     string
		val      float64
		expected bool
	}{
		{"exact zero", 0.0, true},
		{"within epsilon", epsilon / 2, true},
		{"at negative epsilon", -epsilon, true},
		{"larger than epsilon", epsilon * 1.1, false},
		{"one", 1.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsZeroEps(tt.val))
		})
	}
}

func TestIsEqualEps(t *testing.T) {
	tests := []struct {
		name     string
		a        float64
		b        float64
		expected bool
	}{
		{"exactly equal", 1.0, 1.0, true},
		{"within epsilon", 1.0, 1.0 + epsilon/2, true},
		{"larger than epsilon", 1.0, 1.0 + epsilon*1.1, false},
		{"totally different", 1.0, 2.0, false},
		{"zeros", 0.0, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEqualEps(tt.a, tt.b))
		})
	}
}
