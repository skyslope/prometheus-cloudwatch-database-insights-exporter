package cache

import (
	"testing"
	"time"
)

// TestCacheExpirationBasedOnMetricTimestamp verifies that cache expiration
// is based on the metric's measurement timestamp, not the current time.
func TestCacheExpirationBasedOnMetricTimestamp(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Scenario: Metric was measured at second 1
	metricTimestamp := mockTime.Now().Truncate(time.Minute).Add(1 * time.Second)
	ttl := 1 * time.Minute

	// Cache the metric (assume we're fetching it shortly after measurement)
	cache.Set(key, 42.5, metricTimestamp, ttl)

	// Verify entry is cached
	entry, found := cache.Get(key)
	if !found {
		t.Fatal("Expected cache hit immediately after insertion")
	}
	if entry.Value != 42.5 {
		t.Errorf("Expected value 42.5, got %f", entry.Value)
	}

	// Verify expiration is based on metric timestamp
	expectedExpiration := metricTimestamp.Add(ttl) // Second 1 + 60s = Second 61
	if !entry.ExpiresAt.Equal(expectedExpiration) {
		t.Errorf("Expected expiration at %v, got %v", expectedExpiration, entry.ExpiresAt)
	}

	// Advance mock time to second 59 (still within TTL)
	mockTime.currentTime = metricTimestamp.Add(59 * time.Second)
	entry, found = cache.Get(key)
	if !found {
		t.Fatal("Expected cache hit at second 59 (before expiration)")
	}

	// Advance mock time to second 60 (at expiration - should still be valid)
	mockTime.currentTime = metricTimestamp.Add(60 * time.Second)
	entry, found = cache.Get(key)
	if !found {
		t.Error("Expected cache hit at second 60 (at expiration time, still valid)")
	}

	// Advance mock time to second 61 (after expiration - should be expired)
	mockTime.currentTime = metricTimestamp.Add(61 * time.Second)
	_, found = cache.Get(key)
	if found {
		t.Error("Expected cache miss at second 61 (after expiration time)")
	}

	// Verify cache is empty after expiration
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after expiration, got %d", cache.Size())
	}
}

// TestCacheExpirationWithFetchDelay verifies that fetch delay doesn't affect expiration.
func TestCacheExpirationWithFetchDelay(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Metric was measured at second 0
	metricTimestamp := mockTime.Now().Truncate(time.Minute)
	ttl := 1 * time.Minute

	// Simulate 10-second fetch delay - advance time before caching
	mockTime.Advance(10 * time.Second)
	currentTime := mockTime.Now()

	// Cache the metric (fetched 10 seconds after measurement)
	cache.Set(key, 42.5, metricTimestamp, ttl)

	// Expiration should be based on metric timestamp, not fetch time
	expectedExpiration := metricTimestamp.Add(ttl) // Second 0 + 60s = Second 60

	// We're currently at second 10, expiration is at second 60, so should be valid
	entry, found := cache.Get(key)
	if !found {
		t.Fatalf("Expected cache hit at second 10. Current time: %v, Metric time: %v, Expires at: %v",
			currentTime, metricTimestamp, expectedExpiration)
	}
	if !entry.ExpiresAt.Equal(expectedExpiration) {
		t.Errorf("Expected expiration at %v, got %v", expectedExpiration, entry.ExpiresAt)
	}

	// Advance to second 59 from metric timestamp (should still be valid)
	mockTime.currentTime = metricTimestamp.Add(59 * time.Second)
	_, found = cache.Get(key)
	if !found {
		t.Error("Expected cache hit at second 59 from metric timestamp")
	}

	// Advance to second 60 from metric timestamp (at expiration time - should still be valid)
	mockTime.currentTime = metricTimestamp.Add(60 * time.Second)
	_, found = cache.Get(key)
	if !found {
		t.Error("Expected cache hit at second 60 from metric timestamp (at expiration time, still valid)")
	}

	// Advance to second 61 from metric timestamp (after expiration time - should be expired)
	mockTime.currentTime = metricTimestamp.Add(61 * time.Second)
	_, found = cache.Get(key)
	if found {
		t.Error("Expected cache miss at second 61 from metric timestamp (after expiration time)")
	}
}

// TestCacheExpirationWithOldMetric verifies that old metrics expire immediately.
func TestCacheExpirationWithOldMetric(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Metric was measured 2 minutes ago
	metricTimestamp := mockTime.Now().Add(-2 * time.Minute)
	ttl := 1 * time.Minute

	// Cache the old metric
	cache.Set(key, 42.5, metricTimestamp, ttl)

	// Expiration would be: 2 minutes ago + 1 minute = 1 minute ago (already expired!)
	expectedExpiration := metricTimestamp.Add(ttl)

	// Try to get it - should be expired
	_, found := cache.Get(key)
	if found {
		t.Error("Expected cache miss for already-expired metric")
	}

	// Verify the expiration time was set correctly (even though it's in the past)
	// We can't check via Get since it removes expired entries, so check size
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after lazy cleanup of expired entry, got %d", cache.Size())
	}

	t.Logf("Metric timestamp: %v", metricTimestamp)
	t.Logf("Expected expiration: %v", expectedExpiration)
	t.Logf("Current time: %v", mockTime.Now())
}

// TestMultipleMetricsWithDifferentTimestamps verifies that metrics with different
// timestamps expire at different times based on their individual timestamps.
func TestMultipleMetricsWithDifferentTimestamps(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	baseTime := mockTime.Now().Truncate(time.Minute)
	ttl := 1 * time.Minute

	// Insert 3 metrics measured at different times
	key1 := CacheKey{Instance: "instance-1", MetricName: "metric-1", Statistic: "avg"}
	key2 := CacheKey{Instance: "instance-2", MetricName: "metric-2", Statistic: "avg"}
	key3 := CacheKey{Instance: "instance-3", MetricName: "metric-3", Statistic: "avg"}

	// Metric 1: measured at second 0
	cache.Set(key1, 1.0, baseTime, ttl)

	// Metric 2: measured at second 30
	cache.Set(key2, 2.0, baseTime.Add(30*time.Second), ttl)

	// Metric 3: measured at second 45
	cache.Set(key3, 3.0, baseTime.Add(45*time.Second), ttl)

	// At second 59: all should be valid
	mockTime.currentTime = baseTime.Add(59 * time.Second)
	if _, found := cache.Get(key1); !found {
		t.Error("Expected key1 to be valid at second 59")
	}
	if _, found := cache.Get(key2); !found {
		t.Error("Expected key2 to be valid at second 59")
	}
	if _, found := cache.Get(key3); !found {
		t.Error("Expected key3 to be valid at second 59")
	}

	// At second 60: all should still be valid (expires AFTER expiration time, not AT)
	mockTime.currentTime = baseTime.Add(60 * time.Second)
	if _, found := cache.Get(key1); !found {
		t.Error("Expected key1 to still be valid at second 60 (at expiration time)")
	}
	if _, found := cache.Get(key2); !found {
		t.Error("Expected key2 to still be valid at second 60")
	}
	if _, found := cache.Get(key3); !found {
		t.Error("Expected key3 to still be valid at second 60")
	}

	// At second 61: key1 should expire (measured at 0, TTL=60s, expires after 60)
	mockTime.currentTime = baseTime.Add(61 * time.Second)
	if _, found := cache.Get(key1); found {
		t.Error("Expected key1 to be expired at second 61 (after expiration time)")
	}
	if _, found := cache.Get(key2); !found {
		t.Error("Expected key2 to still be valid at second 61")
	}
	if _, found := cache.Get(key3); !found {
		t.Error("Expected key3 to still be valid at second 61")
	}

	// At second 90: key2 should still be valid (measured at 30, TTL=60s, expires after 90)
	mockTime.currentTime = baseTime.Add(90 * time.Second)
	if _, found := cache.Get(key2); !found {
		t.Error("Expected key2 to still be valid at second 90 (at expiration time)")
	}
	if _, found := cache.Get(key3); !found {
		t.Error("Expected key3 to still be valid at second 90")
	}

	// At second 91: key2 should be expired (measured at 30, TTL=60s, expires after 90)
	mockTime.currentTime = baseTime.Add(91 * time.Second)
	if _, found := cache.Get(key2); found {
		t.Error("Expected key2 to be expired at second 91 (after expiration time)")
	}
	if _, found := cache.Get(key3); !found {
		t.Error("Expected key3 to still be valid at second 91")
	}

	// At second 106: all should be expired
	mockTime.currentTime = baseTime.Add(106 * time.Second)
	if _, found := cache.Get(key3); found {
		t.Error("Expected key3 to be expired at second 106")
	}

	// Cache should be empty
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0, got %d", cache.Size())
	}
}
