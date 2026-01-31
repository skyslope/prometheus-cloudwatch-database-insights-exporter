package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// No pattern returns zero (dynamic TTL)
// For any metric data entry without a matching pattern-specific TTL,
// the system should return 0 to indicate dynamic TTL calculation is needed.
func TestProperty_NoPatternReturnsZero(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("metrics without matching patterns return 0 for dynamic TTL", prop.ForAll(
		func(metricName string) bool {
			// Skip empty metric names
			if metricName == "" {
				return true
			}

			// Create TTL policy manager with no patterns
			manager, err := NewTTLPolicyManager([]PatternTTL{})
			if err != nil {
				t.Logf("Failed to create TTL policy manager: %v", err)
				return false
			}

			// Get TTL for any metric name
			actualTTL := manager.GetTTL(metricName)

			// Should return 0 (indicating dynamic TTL needed)
			if actualTTL != 0 {
				t.Logf("Expected 0 TTL (dynamic), got %v for metric %s", actualTTL, metricName)
				return false
			}

			return true
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Pattern-based TTL override
// For any metric data entry whose fully-qualified metric name matches a configured pattern,
// the system should use the pattern's TTL. For non-matching metrics, returns 0 for dynamic TTL.
func TestProperty_PatternBasedTTLOverride(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("metrics matching patterns use pattern-specific TTL, others get 0", prop.ForAll(
		func(patternTTLMinutes int, metricSuffix string) bool {
			// Skip invalid TTL values
			if patternTTLMinutes < 1 || patternTTLMinutes > 1440 {
				return true
			}

			// Skip empty suffixes
			if metricSuffix == "" {
				return true
			}

			patternTTL := time.Duration(patternTTLMinutes) * time.Minute

			// Create a pattern that matches metrics starting with "db."
			patterns := []PatternTTL{
				{
					Pattern: "^db\\.",
					TTL:     patternTTL,
				},
			}

			manager, err := NewTTLPolicyManager(patterns)
			if err != nil {
				t.Logf("Failed to create TTL policy manager: %v", err)
				return false
			}

			// Test metric that matches the pattern
			matchingMetric := fmt.Sprintf("db.%s", metricSuffix)
			actualTTL := manager.GetTTL(matchingMetric)

			if actualTTL != patternTTL {
				t.Logf("Expected pattern TTL %v, got %v for matching metric %s", patternTTL, actualTTL, matchingMetric)
				return false
			}

			// Test metric that doesn't match the pattern - should return 0
			nonMatchingMetric := fmt.Sprintf("os.%s", metricSuffix)
			actualTTL = manager.GetTTL(nonMatchingMetric)

			if actualTTL != 0 {
				t.Logf("Expected 0 TTL (dynamic), got %v for non-matching metric %s", actualTTL, nonMatchingMetric)
				return false
			}

			return true
		},
		gen.IntRange(1, 1440),
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Additional property test: First match wins for multiple patterns
func TestProperty_FirstMatchWins(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("first matching pattern wins when multiple patterns match", prop.ForAll(
		func(firstTTLMinutes int, secondTTLMinutes int, metricSuffix string) bool {
			// Skip invalid TTL values
			if firstTTLMinutes < 1 || firstTTLMinutes > 1440 {
				return true
			}
			if secondTTLMinutes < 1 || secondTTLMinutes > 1440 {
				return true
			}

			// Skip if TTLs are the same (can't distinguish)
			if firstTTLMinutes == secondTTLMinutes {
				return true
			}

			// Skip empty suffixes
			if metricSuffix == "" {
				return true
			}

			firstTTL := time.Duration(firstTTLMinutes) * time.Minute
			secondTTL := time.Duration(secondTTLMinutes) * time.Minute

			// Create two patterns that both match "db." metrics
			// The first pattern should win
			patterns := []PatternTTL{
				{
					Pattern: "^db\\.",
					TTL:     firstTTL,
				},
				{
					Pattern: "^db\\..*",
					TTL:     secondTTL,
				},
			}

			manager, err := NewTTLPolicyManager(patterns)
			if err != nil {
				t.Logf("Failed to create TTL policy manager: %v", err)
				return false
			}

			// Test metric that matches both patterns
			matchingMetric := fmt.Sprintf("db.%s", metricSuffix)
			actualTTL := manager.GetTTL(matchingMetric)

			// Should use the first pattern's TTL
			if actualTTL != firstTTL {
				t.Logf("Expected first pattern TTL %v, got %v for metric %s", firstTTL, actualTTL, matchingMetric)
				return false
			}

			return true
		},
		gen.IntRange(1, 1440),
		gen.IntRange(1, 1440),
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
