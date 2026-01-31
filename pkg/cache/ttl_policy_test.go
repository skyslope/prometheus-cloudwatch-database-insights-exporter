package cache

import (
	"testing"
	"time"
)

func TestTTLPolicyManager_NoPatternReturnsZero(t *testing.T) {
	manager, err := NewTTLPolicyManager([]PatternTTL{})
	if err != nil {
		t.Fatalf("Failed to create TTL policy manager: %v", err)
	}

	// Test various metric names - all should return 0 (indicating dynamic TTL needed)
	testCases := []string{
		"db.load.avg",
		"os.cpuUtilization.total.avg",
		"db.Transactions.xact_commit.avg",
		"random.metric.name",
	}

	for _, metricName := range testCases {
		ttl := manager.GetTTL(metricName)
		if ttl != 0 {
			t.Errorf("Expected 0 TTL (dynamic) for metric %s with no patterns, got %v", metricName, ttl)
		}
	}
}

func TestTTLPolicyManager_PatternOverride(t *testing.T) {
	dbTTL := 10 * time.Minute
	osTTL := 2 * time.Minute

	patterns := []PatternTTL{
		{Pattern: "^db\\.", TTL: dbTTL},
		{Pattern: "^os\\.", TTL: osTTL},
	}

	manager, err := NewTTLPolicyManager(patterns)
	if err != nil {
		t.Fatalf("Failed to create TTL policy manager: %v", err)
	}

	testCases := []struct {
		metricName  string
		expectedTTL time.Duration
	}{
		{"db.load.avg", dbTTL},
		{"db.Transactions.xact_commit.avg", dbTTL},
		{"os.cpuUtilization.total.avg", osTTL},
		{"os.memory.free", osTTL},
		{"other.metric.name", 0}, // No pattern match = dynamic TTL
	}

	for _, tc := range testCases {
		ttl := manager.GetTTL(tc.metricName)
		if ttl != tc.expectedTTL {
			t.Errorf("Expected TTL %v for metric %s, got %v", tc.expectedTTL, tc.metricName, ttl)
		}
	}
}

func TestTTLPolicyManager_FirstMatchWins(t *testing.T) {
	firstTTL := 10 * time.Minute
	secondTTL := 2 * time.Minute

	// Both patterns match "db." metrics, but first should win
	patterns := []PatternTTL{
		{Pattern: "^db\\.", TTL: firstTTL},
		{Pattern: "^db\\..*", TTL: secondTTL},
	}

	manager, err := NewTTLPolicyManager(patterns)
	if err != nil {
		t.Fatalf("Failed to create TTL policy manager: %v", err)
	}

	// Should use first pattern's TTL
	ttl := manager.GetTTL("db.load.avg")
	if ttl != firstTTL {
		t.Errorf("Expected first pattern TTL %v, got %v", firstTTL, ttl)
	}
}

func TestTTLPolicyManager_InvalidPattern(t *testing.T) {
	// Invalid regex pattern
	patterns := []PatternTTL{
		{Pattern: "[invalid(", TTL: 10 * time.Minute},
	}

	_, err := NewTTLPolicyManager(patterns)
	if err == nil {
		t.Error("Expected error for invalid regex pattern, got nil")
	}
}

func TestTTLPolicyManager_ComplexPatterns(t *testing.T) {
	patterns := []PatternTTL{
		// Specific pattern for transaction metrics
		{Pattern: "^db\\.Transactions\\.", TTL: 1 * time.Minute},
		// General pattern for all db metrics
		{Pattern: "^db\\.", TTL: 10 * time.Minute},
		// Pattern for OS CPU metrics
		{Pattern: "^os\\.cpu", TTL: 30 * time.Second},
		// General pattern for all OS metrics
		{Pattern: "^os\\.", TTL: 2 * time.Minute},
	}

	manager, err := NewTTLPolicyManager(patterns)
	if err != nil {
		t.Fatalf("Failed to create TTL policy manager: %v", err)
	}

	testCases := []struct {
		metricName  string
		expectedTTL time.Duration
	}{
		// Should match first pattern (most specific)
		{"db.Transactions.xact_commit.avg", 1 * time.Minute},
		// Should match second pattern
		{"db.load.avg", 10 * time.Minute},
		// Should match third pattern (most specific)
		{"os.cpuUtilization.total.avg", 30 * time.Second},
		// Should match fourth pattern
		{"os.memory.free", 2 * time.Minute},
		// Should return 0 (dynamic TTL)
		{"other.metric", 0},
	}

	for _, tc := range testCases {
		ttl := manager.GetTTL(tc.metricName)
		if ttl != tc.expectedTTL {
			t.Errorf("Expected TTL %v for metric %s, got %v", tc.expectedTTL, tc.metricName, ttl)
		}
	}
}

// TestTTLPolicyManager_FullyQualifiedNameMatching tests pattern matching with metric names
func TestTTLPolicyManager_MetricNameMatching(t *testing.T) {
	patterns := []PatternTTL{
		{Pattern: "^db\\.load\\..*", TTL: 30 * time.Second},
		{Pattern: "^os\\.memory\\..*", TTL: 5 * time.Minute},
		{Pattern: "^db\\.SQL\\..*\\.sum$", TTL: 1 * time.Minute},
		{Pattern: ".*\\.max$", TTL: 2 * time.Minute},
	}

	manager, err := NewTTLPolicyManager(patterns)
	if err != nil {
		t.Fatalf("Failed to create TTL policy manager: %v", err)
	}

	testCases := []struct {
		name        string
		metricName  string
		expectedTTL time.Duration
		description string
	}{
		{
			name:        "Match db.load pattern",
			metricName:  "db.load.avg",
			expectedTTL: 30 * time.Second,
			description: "Should match ^db\\.load\\..* pattern",
		},
		{
			name:        "Match os.memory pattern",
			metricName:  "os.memory.total.avg",
			expectedTTL: 5 * time.Minute,
			description: "Should match ^os\\.memory\\..* pattern",
		},
		{
			name:        "Match db.SQL with sum statistic",
			metricName:  "db.SQL.queries.sum",
			expectedTTL: 1 * time.Minute,
			description: "Should match ^db\\.SQL\\..*\\.sum$ pattern",
		},
		{
			name:        "Match max statistic pattern",
			metricName:  "os.cpuUtilization.user.max",
			expectedTTL: 2 * time.Minute,
			description: "Should match .*\\.max$ pattern",
		},
		{
			name:        "No pattern match - returns 0",
			metricName:  "custom.metric.avg",
			expectedTTL: 0,
			description: "No pattern matches, should return 0 for dynamic TTL",
		},
		{
			name:        "First match wins",
			metricName:  "db.load.avg.max",
			expectedTTL: 30 * time.Second,
			description: "Matches both ^db\\.load\\..* (30s) and .*\\.max$ (2m), first wins",
		},
		{
			name:        "db.SQL with avg doesn't match sum pattern",
			metricName:  "db.SQL.queries.avg",
			expectedTTL: 0,
			description: "Doesn't match ^db\\.SQL\\..*\\.sum$ because statistic is avg, not sum",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ttl := manager.GetTTL(tc.metricName)
			if ttl != tc.expectedTTL {
				t.Errorf("%s: expected TTL %v, got %v", tc.description, tc.expectedTTL, ttl)
			}
		})
	}
}
