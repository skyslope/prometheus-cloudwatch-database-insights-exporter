package utils

import (
	"testing"
)

func TestIsDebugEnabled(t *testing.T) {
	tests := []struct {
		name     string
		setValue bool
		expected bool
	}{
		{
			name:     "debug enabled",
			setValue: true,
			expected: true,
		},
		{
			name:     "debug disabled",
			setValue: false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set debug flag
			SetDebugEnabled(tt.setValue)

			// Test
			result := IsDebugEnabled()
			if result != tt.expected {
				t.Errorf("Expected IsDebugEnabled() = %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSetDebugEnabled(t *testing.T) {
	// Test setting to true
	SetDebugEnabled(true)
	if !IsDebugEnabled() {
		t.Error("Expected debug to be enabled after SetDebugEnabled(true)")
	}

	// Test setting to false
	SetDebugEnabled(false)
	if IsDebugEnabled() {
		t.Error("Expected debug to be disabled after SetDebugEnabled(false)")
	}
}
