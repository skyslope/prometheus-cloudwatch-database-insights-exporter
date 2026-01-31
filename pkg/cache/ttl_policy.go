package cache

import (
	"regexp"
	"time"
)

// TTLPolicyManager manages TTL policies for metric data cache entries.
// It supports pattern-based TTL overrides with ordered pattern matching.
// Patterns match against metric names (e.g., "db.load.avg", "os.memory.total.avg").
// If no pattern matches, returns zero duration to indicate dynamic TTL calculation is needed.
type TTLPolicyManager interface {
	// GetTTL returns the TTL for a given metric name.
	// It matches the metric name against configured patterns in order,
	// returning the first matching pattern's TTL.
	// If no pattern matches, returns 0 to indicate dynamic TTL calculation should be used.
	GetTTL(metricName string) time.Duration
}

// ttlPolicyManager implements TTLPolicyManager with compiled regex patterns.
type ttlPolicyManager struct {
	patternPolicies []patternPolicy
}

// patternPolicy represents a single pattern-based TTL policy.
type patternPolicy struct {
	pattern *regexp.Regexp
	ttl     time.Duration
}

// NewTTLPolicyManager creates a new TTL policy manager with pattern-based TTL overrides.
// If no patterns are provided, all metrics will use dynamic TTL calculation.
func NewTTLPolicyManager(patterns []PatternTTL) (TTLPolicyManager, error) {
	policies := make([]patternPolicy, 0, len(patterns))

	for _, p := range patterns {
		compiled, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, err
		}

		policies = append(policies, patternPolicy{
			pattern: compiled,
			ttl:     p.TTL,
		})
	}

	return &ttlPolicyManager{
		patternPolicies: policies,
	}, nil
}

// GetTTL returns the TTL for a given metric name.
// Returns the first matching pattern's TTL, or 0 if no pattern matches
// (indicating dynamic TTL calculation should be used).
func (m *ttlPolicyManager) GetTTL(metricName string) time.Duration {
	// Try to match against patterns in order (first match wins)
	for _, policy := range m.patternPolicies {
		if policy.pattern.MatchString(metricName) {
			return policy.ttl
		}
	}

	// No pattern matched, return 0 to indicate dynamic TTL calculation needed
	return 0
}

// PatternTTL represents a pattern-based TTL configuration.
type PatternTTL struct {
	Pattern string
	TTL     time.Duration
}
