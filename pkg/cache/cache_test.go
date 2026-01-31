package cache

import (
	"testing"
	"time"
)

// TestCacheHitBeforeExpiration tests that cache returns values before TTL expires.
func TestCacheHitBeforeExpiration(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Insert entry with 5 minute TTL
	cache.Set(key, 42.5, mockTime.Now(), 5*time.Minute)

	// Immediately retrieve - should be a hit
	entry, found := cache.Get(key)
	if !found {
		t.Fatal("Expected cache hit immediately after insertion")
	}
	if entry.Value != 42.5 {
		t.Errorf("Expected value 42.5, got %f", entry.Value)
	}

	// Advance time by 4 minutes (still within TTL)
	mockTime.Advance(4 * time.Minute)

	// Should still be a hit
	entry, found = cache.Get(key)
	if !found {
		t.Fatal("Expected cache hit before TTL expiration")
	}
	if entry.Value != 42.5 {
		t.Errorf("Expected value 42.5, got %f", entry.Value)
	}
}

// TestCacheMissAfterExpiration tests that cache returns miss after TTL expires.
func TestCacheMissAfterExpiration(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Insert entry with 5 minute TTL
	cache.Set(key, 42.5, mockTime.Now(), 5*time.Minute)

	// Advance time past TTL
	mockTime.Advance(6 * time.Minute)

	// Should be a miss
	_, found := cache.Get(key)
	if found {
		t.Fatal("Expected cache miss after TTL expiration")
	}

	// Verify entry was removed from cache (lazy cleanup)
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after lazy cleanup, got %d", cache.Size())
	}
}

// TestLazyEvictionTrigger tests that lazy eviction is triggered when cache reaches max size.
func TestLazyEvictionTrigger(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	maxSize := 5
	cache := NewMetricCache(maxSize, mockTime)

	// Fill cache with entries that will expire
	for i := 0; i < maxSize; i++ {
		key := CacheKey{
			Instance:   "instance-" + string(rune('A'+i)),
			MetricName: "test.metric",
			Statistic:  "avg",
		}
		cache.Set(key, float64(i), mockTime.Now(), 1*time.Minute)
	}

	// Verify cache is full
	if cache.Size() != maxSize {
		t.Fatalf("Expected cache size %d, got %d", maxSize, cache.Size())
	}

	// Advance time to expire all entries
	mockTime.Advance(2 * time.Minute)

	// Insert a new entry - should trigger lazy eviction
	newKey := CacheKey{
		Instance:   "new-instance",
		MetricName: "test.metric",
		Statistic:  "avg",
	}
	cache.Set(newKey, 999.0, mockTime.Now(), 1*time.Hour)

	// Cache should have evicted expired entries
	// Only the new entry should remain
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1 after lazy eviction, got %d", cache.Size())
	}

	// Verify new entry is present
	entry, found := cache.Get(newKey)
	if !found {
		t.Fatal("Expected new entry to be present after lazy eviction")
	}
	if entry.Value != 999.0 {
		t.Errorf("Expected value 999.0, got %f", entry.Value)
	}
}

// TestEvictOldestWhenNoExpiredEntries tests that when cache is full and no entries
// are expired, the oldest entry (by expiration time) is evicted.
func TestEvictOldestWhenNoExpiredEntries(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	maxSize := 3
	cache := NewMetricCache(maxSize, mockTime)

	// Insert entries with different TTLs (all valid)
	key1 := CacheKey{Instance: "instance-1", MetricName: "test", Statistic: "avg"}
	key2 := CacheKey{Instance: "instance-2", MetricName: "test", Statistic: "avg"}
	key3 := CacheKey{Instance: "instance-3", MetricName: "test", Statistic: "avg"}

	cache.Set(key1, 1.0, mockTime.Now(), 1*time.Hour) // Expires first
	cache.Set(key2, 2.0, mockTime.Now(), 2*time.Hour) // Expires second
	cache.Set(key3, 3.0, mockTime.Now(), 3*time.Hour) // Expires last

	// Cache is full
	if cache.Size() != maxSize {
		t.Fatalf("Expected cache size %d, got %d", maxSize, cache.Size())
	}

	// Insert a new entry - should evict key1 (oldest expiration)
	key4 := CacheKey{Instance: "instance-4", MetricName: "test", Statistic: "avg"}
	cache.Set(key4, 4.0, mockTime.Now(), 4*time.Hour)

	// Cache should still be at max size
	if cache.Size() != maxSize {
		t.Errorf("Expected cache size %d, got %d", maxSize, cache.Size())
	}

	// key1 should be evicted
	_, found := cache.Get(key1)
	if found {
		t.Error("Expected key1 to be evicted")
	}

	// Other keys should still be present
	_, found = cache.Get(key2)
	if !found {
		t.Error("Expected key2 to still be present")
	}
	_, found = cache.Get(key3)
	if !found {
		t.Error("Expected key3 to still be present")
	}
	_, found = cache.Get(key4)
	if !found {
		t.Error("Expected key4 to be present")
	}
}

// TestExplicitEviction tests the EvictExpired method.
func TestExplicitEviction(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	// Insert 5 entries that will expire
	for i := 0; i < 5; i++ {
		key := CacheKey{
			Instance:   "expired-" + string(rune('A'+i)),
			MetricName: "test.metric",
			Statistic:  "avg",
		}
		cache.Set(key, float64(i), mockTime.Now(), 1*time.Minute)
	}

	// Insert 3 entries that won't expire
	for i := 0; i < 3; i++ {
		key := CacheKey{
			Instance:   "valid-" + string(rune('A'+i)),
			MetricName: "test.metric",
			Statistic:  "avg",
		}
		cache.Set(key, float64(i+100), mockTime.Now(), 1*time.Hour)
	}

	// Verify cache has 8 entries
	if cache.Size() != 8 {
		t.Fatalf("Expected cache size 8, got %d", cache.Size())
	}

	// Advance time to expire first batch
	mockTime.Advance(2 * time.Minute)

	// Explicitly evict up to 10 expired entries
	evicted := cache.EvictExpired(10)

	// Should have evicted 5 entries
	if evicted != 5 {
		t.Errorf("Expected to evict 5 entries, got %d", evicted)
	}

	// Cache should have 3 entries remaining
	if cache.Size() != 3 {
		t.Errorf("Expected cache size 3, got %d", cache.Size())
	}
}

// TestUpdateExistingEntry tests that updating an existing entry works correctly.
func TestUpdateExistingEntry(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Insert initial entry
	cache.Set(key, 10.0, mockTime.Now(), 5*time.Minute)

	// Verify initial value
	entry, found := cache.Get(key)
	if !found || entry.Value != 10.0 {
		t.Fatal("Expected initial value 10.0")
	}

	// Update entry with new value and TTL
	mockTime.Advance(1 * time.Minute)
	cache.Set(key, 20.0, mockTime.Now(), 10*time.Minute)

	// Verify updated value
	entry, found = cache.Get(key)
	if !found || entry.Value != 20.0 {
		t.Fatalf("Expected updated value 20.0, got %f", entry.Value)
	}

	// Advance time past original TTL but before new TTL
	mockTime.Advance(5 * time.Minute)

	// Entry should still be valid (new TTL is longer)
	entry, found = cache.Get(key)
	if !found {
		t.Fatal("Expected entry to still be valid with new TTL")
	}
	if entry.Value != 20.0 {
		t.Errorf("Expected value 20.0, got %f", entry.Value)
	}

	// Cache size should still be 1 (not 2)
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1 after update, got %d", cache.Size())
	}
}

// TestConcurrentAccess tests basic concurrent access patterns.
func TestConcurrentAccess(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(100, mockTime)

	// This is a basic smoke test for concurrent access
	// More thorough concurrency testing would require race detector
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 50; i++ {
			key := CacheKey{
				Instance:   "instance-writer",
				MetricName: "test.metric",
				Statistic:  "avg",
			}
			cache.Set(key, float64(i), mockTime.Now(), 1*time.Hour)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 50; i++ {
			key := CacheKey{
				Instance:   "instance-writer",
				MetricName: "test.metric",
				Statistic:  "avg",
			}
			cache.Get(key)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we get here without deadlock or panic, test passes
}

// TestEmptyCache tests operations on an empty cache.
func TestEmptyCache(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	// Size should be 0
	if cache.Size() != 0 {
		t.Errorf("Expected empty cache size 0, got %d", cache.Size())
	}

	// Get on empty cache should return miss
	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "test.metric",
		Statistic:  "avg",
	}
	_, found := cache.Get(key)
	if found {
		t.Error("Expected cache miss on empty cache")
	}

	// Evict on empty cache should return 0
	evicted := cache.EvictExpired(10)
	if evicted != 0 {
		t.Errorf("Expected 0 evictions on empty cache, got %d", evicted)
	}
}

// TestTimestampPreservation tests that original timestamps are preserved.
func TestTimestampPreservation(t *testing.T) {
	mockTime := &MockTimeProvider{currentTime: time.Now()}
	cache := NewMetricCache(10, mockTime)

	key := CacheKey{
		Instance:   "test-instance",
		MetricName: "db.load.avg",
		Statistic:  "avg",
	}

	// Use a recent timestamp (within the last minute) so it won't be expired
	originalTimestamp := mockTime.Now().Add(-30 * time.Second)

	// Insert entry with specific timestamp and 5 minute TTL
	cache.Set(key, 42.5, originalTimestamp, 5*time.Minute)

	// Retrieve entry
	entry, found := cache.Get(key)
	if !found {
		t.Fatal("Expected cache hit")
	}

	// Verify timestamp is preserved
	if !entry.Timestamp.Equal(originalTimestamp) {
		t.Errorf("Expected timestamp %v, got %v", originalTimestamp, entry.Timestamp)
	}

	// Verify expiration is based on metric timestamp, not current time
	expectedExpiration := originalTimestamp.Add(5 * time.Minute)
	if !entry.ExpiresAt.Equal(expectedExpiration) {
		t.Errorf("Expected expiration %v, got %v", expectedExpiration, entry.ExpiresAt)
	}
}
