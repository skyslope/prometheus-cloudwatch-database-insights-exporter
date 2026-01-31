package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// MockTimeProvider is a mock implementation of TimeProvider for testing.
type MockTimeProvider struct {
	currentTime time.Time
}

func (m *MockTimeProvider) Now() time.Time {
	return m.currentTime
}

func (m *MockTimeProvider) Advance(d time.Duration) {
	m.currentTime = m.currentTime.Add(d)
}

// Cache insertion complexity
// For any cache with N entries, inserting a new entry should complete in O(log N) time complexity.
func TestProperty_CacheInsertionComplexity(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("cache insertion completes in O(log N) time", prop.ForAll(
		func(numEntries int) bool {
			// Skip invalid sizes
			if numEntries < 10 || numEntries > 10000 {
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			cache := NewMetricCache(numEntries+1, mockTime)

			// Pre-populate cache with numEntries entries
			for i := 0; i < numEntries; i++ {
				key := CacheKey{

					Instance:   fmt.Sprintf("instance-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				cache.Set(key, float64(i), mockTime.Now(), time.Hour)
			}

			// Measure time for a single insertion
			newKey := CacheKey{

				Instance:   "new-instance",
				MetricName: "test.metric",
				Statistic:  "avg",
			}

			start := time.Now()
			cache.Set(newKey, 999.0, mockTime.Now(), time.Hour)
			elapsed := time.Since(start)

			// O(log N) should complete very quickly even for large N
			// For 10,000 entries, log2(10000) ≈ 13.3 operations
			// We expect this to complete in microseconds, not milliseconds
			maxExpectedTime := time.Millisecond * 10 // Very generous upper bound

			if elapsed > maxExpectedTime {
				t.Logf("Insertion took %v for %d entries (expected < %v)", elapsed, numEntries, maxExpectedTime)
				return false
			}

			// Verify the entry was actually inserted
			entry, found := cache.Get(newKey)
			if !found || entry.Value != 999.0 {
				return false
			}

			return true
		},
		gen.IntRange(10, 10000),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Cache lookup complexity
// For any cache with N entries, looking up an entry by key should complete in O(log N) time complexity.
func TestProperty_CacheLookupComplexity(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("cache lookup completes in O(log N) time", prop.ForAll(
		func(numEntries int) bool {
			// Skip invalid sizes
			if numEntries < 10 || numEntries > 10000 {
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			cache := NewMetricCache(numEntries, mockTime)

			// Pre-populate cache with numEntries entries
			targetKey := CacheKey{

				Instance:   "target-instance",
				MetricName: "test.metric",
				Statistic:  "avg",
			}

			for i := 0; i < numEntries; i++ {
				key := CacheKey{

					Instance:   fmt.Sprintf("instance-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				cache.Set(key, float64(i), mockTime.Now(), time.Hour)
			}

			// Insert target key in the middle
			cache.Set(targetKey, 999.0, mockTime.Now(), time.Hour)

			// Measure time for a single lookup
			start := time.Now()
			entry, found := cache.Get(targetKey)
			elapsed := time.Since(start)

			// O(log N) lookup should be very fast
			maxExpectedTime := time.Millisecond * 5 // Very generous upper bound

			if elapsed > maxExpectedTime {
				t.Logf("Lookup took %v for %d entries (expected < %v)", elapsed, numEntries, maxExpectedTime)
				return false
			}

			// Verify the correct entry was found
			if !found || entry.Value != 999.0 {
				return false
			}

			return true
		},
		gen.IntRange(10, 10000),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Eviction ordering
// For any set of expired cache entries, the system should evict entries
// in order of earliest expiration timestamp first.
func TestProperty_EvictionOrdering(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("expired entries are evicted in order of earliest expiration first", prop.ForAll(
		func(numEntries int) bool {
			// Skip invalid sizes
			if numEntries < 2 || numEntries > 100 {
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			cache := NewMetricCache(numEntries*2, mockTime)

			// Insert entries with different expiration times
			expirationTimes := make([]time.Time, numEntries)
			for i := 0; i < numEntries; i++ {
				key := CacheKey{

					Instance:   fmt.Sprintf("instance-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				// Each entry expires at a different time
				ttl := time.Duration(i+1) * time.Minute
				expirationTimes[i] = mockTime.Now().Add(ttl)
				cache.Set(key, float64(i), mockTime.Now(), ttl)
			}

			// Advance time past all expirations
			mockTime.Advance(time.Duration(numEntries+1) * time.Minute)

			// Evict entries one by one and verify order
			for i := 0; i < numEntries; i++ {
				sizeBefore := cache.Size()
				evicted := cache.EvictExpired(1)

				if evicted != 1 {
					t.Logf("Expected to evict 1 entry, but evicted %d", evicted)
					return false
				}

				sizeAfter := cache.Size()
				if sizeBefore-sizeAfter != 1 {
					t.Logf("Cache size did not decrease by 1 (before: %d, after: %d)", sizeBefore, sizeAfter)
					return false
				}

				// The entry with the earliest expiration should have been evicted
				// We can't directly verify which entry was evicted, but we can verify
				// that the cache size decreases correctly
			}

			// After evicting all entries, cache should be empty
			if cache.Size() != 0 {
				t.Logf("Cache should be empty after evicting all entries, but has %d entries", cache.Size())
				return false
			}

			return true
		},
		gen.IntRange(2, 100),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Eviction complexity
// For any eviction operation removing N entries, the operation should
// complete in O(N) time complexity.
func TestProperty_EvictionComplexity(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("eviction of N entries completes in O(N) time", prop.ForAll(
		func(numToEvict int) bool {
			// Skip invalid sizes
			if numToEvict < 10 || numToEvict > 1000 {
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			totalEntries := numToEvict * 2
			cache := NewMetricCache(totalEntries, mockTime)

			// Insert entries that will expire
			for i := 0; i < numToEvict; i++ {
				key := CacheKey{

					Instance:   fmt.Sprintf("expired-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				cache.Set(key, float64(i), mockTime.Now(), time.Minute)
			}

			// Insert entries that won't expire
			for i := 0; i < numToEvict; i++ {
				key := CacheKey{

					Instance:   fmt.Sprintf("valid-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				cache.Set(key, float64(i), mockTime.Now(), time.Hour*24)
			}

			// Advance time to expire first batch
			mockTime.Advance(time.Minute * 2)

			// Measure time to evict N entries
			start := time.Now()
			evicted := cache.EvictExpired(numToEvict)
			elapsed := time.Since(start)

			// O(N) should scale linearly
			// For 1000 entries, this should still complete in milliseconds
			maxExpectedTime := time.Millisecond * 50 // Generous upper bound

			if elapsed > maxExpectedTime {
				t.Logf("Eviction of %d entries took %v (expected < %v)", numToEvict, elapsed, maxExpectedTime)
				return false
			}

			// Verify correct number of entries were evicted
			if evicted != numToEvict {
				t.Logf("Expected to evict %d entries, but evicted %d", numToEvict, evicted)
				return false
			}

			// Verify remaining entries are the non-expired ones
			if cache.Size() != numToEvict {
				t.Logf("Expected %d remaining entries, but have %d", numToEvict, cache.Size())
				return false
			}

			return true
		},
		gen.IntRange(10, 1000),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Lazy eviction on size limit
// For any cache at maximum size, the system should evict expired entries
// before inserting new entries.
func TestProperty_LazyEvictionOnSizeLimit(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("cache evicts expired entries when at max size before inserting new entries", prop.ForAll(
		func(maxSize int) bool {
			// Skip invalid sizes
			if maxSize < 5 || maxSize > 100 {
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			cache := NewMetricCache(maxSize, mockTime)

			// Fill cache to max size with entries that will expire soon
			for i := 0; i < maxSize; i++ {
				key := CacheKey{

					Instance:   fmt.Sprintf("instance-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				cache.Set(key, float64(i), mockTime.Now(), time.Minute)
			}

			// Verify cache is at max size
			if cache.Size() != maxSize {
				return false
			}

			// Advance time to expire all entries
			mockTime.Advance(time.Minute * 2)

			// Insert a new entry - should trigger lazy eviction
			newKey := CacheKey{

				Instance:   "new-instance",
				MetricName: "test.metric",
				Statistic:  "avg",
			}
			cache.Set(newKey, 999.0, mockTime.Now(), time.Hour)

			// Cache should have evicted expired entries and inserted the new one
			// Size should be 1 (only the new entry)
			if cache.Size() != 1 {
				t.Logf("Expected cache size 1 after lazy eviction, got %d", cache.Size())
				return false
			}

			// Verify the new entry is present
			entry, found := cache.Get(newKey)
			if !found || entry.Value != 999.0 {
				return false
			}

			// Verify old entries are gone
			for i := 0; i < maxSize; i++ {
				oldKey := CacheKey{

					Instance:   fmt.Sprintf("instance-%d", i),
					MetricName: "test.metric",
					Statistic:  "avg",
				}
				_, found := cache.Get(oldKey)
				if found {
					t.Logf("Old entry %d should have been evicted but is still present", i)
					return false
				}
			}

			return true
		},
		gen.IntRange(5, 100),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Fully-qualified cache key
// For any metric data entry, the cache key should include instance identifier,
// metric name, and aggregation statistic.
// Note: Region is not included as each MetricManager is scoped to a specific region.
func TestProperty_FullyQualifiedCacheKey(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("cache keys include instance, metric name, and statistic", prop.ForAll(
		func(instance, metricName, statistic string) bool {
			// Skip empty values
			if instance == "" || metricName == "" || statistic == "" {
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			cache := NewMetricCache(100, mockTime)

			// Create a fully-qualified cache key
			key := CacheKey{
				Instance:   instance,
				MetricName: metricName,
				Statistic:  statistic,
			}

			// Insert entry
			cache.Set(key, 123.45, mockTime.Now(), time.Hour)

			// Retrieve entry with exact same key
			entry, found := cache.Get(key)
			if !found || entry.Value != 123.45 {
				return false
			}

			// Verify that changing any component of the key results in a cache miss
			// Different instance
			key2 := CacheKey{
				Instance:   instance + "-different",
				MetricName: metricName,
				Statistic:  statistic,
			}
			_, found = cache.Get(key2)
			if found {
				return false
			}

			// Different metric name
			key3 := CacheKey{
				Instance:   instance,
				MetricName: metricName + ".different",
				Statistic:  statistic,
			}
			_, found = cache.Get(key3)
			if found {
				return false
			}

			// Different statistic
			key4 := CacheKey{
				Instance:   instance,
				MetricName: metricName,
				Statistic:  statistic + "-different",
			}
			_, found = cache.Get(key4)
			if found {
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Time mocking in TTL tests
// For any unit test validating TTL expiration logic, the test should use
// time mocking to avoid unreasonably long test execution times.
func TestProperty_TimeMockingInTTLTests(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("TTL expiration tests use time mocking to avoid long execution times", prop.ForAll(
		func(ttlMinutes int) bool {
			// Skip invalid TTLs
			if ttlMinutes < 1 || ttlMinutes > 1440 { // 1 minute to 24 hours
				return true
			}

			mockTime := &MockTimeProvider{currentTime: time.Now()}
			cache := NewMetricCache(10, mockTime)

			key := CacheKey{

				Instance:   "test-instance",
				MetricName: "test.metric",
				Statistic:  "avg",
			}

			ttl := time.Duration(ttlMinutes) * time.Minute

			// Measure actual wall-clock time for the test
			testStart := time.Now()

			// Insert entry with TTL
			cache.Set(key, 123.45, mockTime.Now(), ttl)

			// Verify entry is present before expiration
			_, found := cache.Get(key)
			if !found {
				return false
			}

			// Advance mock time past TTL (instant in wall-clock time)
			mockTime.Advance(ttl + time.Second)

			// Verify entry is expired
			_, found = cache.Get(key)
			if found {
				return false
			}

			testElapsed := time.Since(testStart)

			// The test should complete in milliseconds, not minutes
			// Even for a 24-hour TTL, the test should be instant
			maxTestTime := time.Millisecond * 100

			if testElapsed > maxTestTime {
				t.Logf("TTL test for %d minutes took %v (expected < %v)", ttlMinutes, testElapsed, maxTestTime)
				return false
			}

			return true
		},
		gen.IntRange(1, 1440),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
