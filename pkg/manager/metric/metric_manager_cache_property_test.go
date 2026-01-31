package metric

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/cache"
	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/testutils"
	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/testutils/mocks"
)

// Cache consultation before fetch
// For any metric collection operation, the system should exclude metrics with valid non-expired cache entries from the fetch list.
func TestProperty_CacheConsultationBeforeFetch(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("metrics with valid cache entries are excluded from fetch list", prop.ForAll(
		func(numMetrics int, numCached int) bool {
			// Skip invalid inputs
			if numMetrics < 1 || numMetrics > 20 || numCached < 0 || numCached > numMetrics {
				return true
			}

			// Create metric names
			metricNames := make([]string, numMetrics)
			for i := 0; i < numMetrics; i++ {
				metricNames[i] = fmt.Sprintf("os.metric%d.avg", i)
			}

			// Create instance
			instance := testutils.NewTestInstancePostgreSQL()

			// Create metric manager with cache
			mockPI := &mocks.MockPIService{}
			manager, err := NewMetricManager(mockPI, testutils.CreateDefaultParsedTestConfig(), testutils.TestRegion)
			if err != nil {
				return false
			}

			// Pre-populate cache with some metrics
			now := time.Now()
			for i := 0; i < numCached; i++ {
				cacheKey := cache.CacheKey{

					Instance:   instance.Identifier,
					MetricName: metricNames[i],
					Statistic:  "avg",
				}
				// Set with a long TTL so they don't expire during the test
				manager.metricDataCache.Set(cacheKey, float64(i*10), now, 10*time.Minute)
			}

			// Call filterCachedMetrics
			metricsToFetch, cachedMetrics := manager.filterCachedMetrics(instance, metricNames)

			// Property: Metrics with valid cache entries should be excluded from fetch list
			// The number of metrics to fetch should be (total - cached)
			expectedToFetch := numMetrics - numCached
			if len(metricsToFetch) != expectedToFetch {
				return false
			}

			// Property: Cached metrics should be returned
			if len(cachedMetrics) != numCached {
				return false
			}

			// Property: No metric should appear in both lists
			fetchSet := make(map[string]bool)
			for _, m := range metricsToFetch {
				fetchSet[m] = true
			}
			for _, m := range cachedMetrics {
				if fetchSet[m.Metric] {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 20),
		gen.IntRange(0, 20),
	))

	properties.TestingRun(t)
}

// Cache update after fetch
// For any successful metric fetch from AWS, the system should update the cache with the new value using the TTL determined by pattern matching.
func TestProperty_CacheUpdateAfterFetch(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("cache is updated after successful fetch with pattern-based TTL", prop.ForAll(
		func(numMetrics int) bool {
			// Skip invalid inputs
			if numMetrics < 1 || numMetrics > 10 {
				return true
			}

			// Create instance
			instance := testutils.NewTestInstancePostgreSQL()

			// Create metric manager with cache
			mockPI := &mocks.MockPIService{}
			manager, err := NewMetricManager(mockPI, testutils.CreateDefaultParsedTestConfig(), testutils.TestRegion)
			if err != nil {
				return false
			}

			// Create metric data to update
			now := time.Now()
			for i := 0; i < numMetrics; i++ {
				metricDatum := testutils.NewTestMetricData(
					fmt.Sprintf("os.metric%d.avg", i),
					float64(i*100),
				)
				metricDatum.Timestamp = now

				// Update cache with 1 minute dynamic TTL (simulating calculated TTL)
				manager.updateCacheWithDynamicTTL(instance, metricDatum, 1*time.Minute)

				// Verify cache was updated
				cacheKey := cache.CacheKey{

					Instance:   instance.Identifier,
					MetricName: metricDatum.Metric,
					Statistic:  "avg",
				}

				entry, found := manager.metricDataCache.Get(cacheKey)
				if !found {
					return false
				}

				// Verify value matches
				if entry.Value != metricDatum.Value {
					return false
				}

				// Verify timestamp matches
				if !entry.Timestamp.Equal(metricDatum.Timestamp) {
					return false
				}

				// Verify TTL was applied (entry should not be expired immediately)
				if time.Now().After(entry.ExpiresAt) {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// Cache hit serving
// For any scrape request, the system should return cached values for cache hits without fetching from AWS.
func TestProperty_CacheHitServing(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("cached values are served without AWS fetch", prop.ForAll(
		func(numMetrics int, numCached int) bool {
			// Skip invalid inputs
			if numMetrics < 1 || numMetrics > 15 || numCached < 0 || numCached > numMetrics {
				return true
			}

			// Create metric names
			metricNames := make([]string, numMetrics)
			for i := 0; i < numMetrics; i++ {
				metricNames[i] = fmt.Sprintf("db.metric%d.avg", i)
			}

			// Create instance
			instance := testutils.NewTestInstancePostgreSQL()

			// Create metric manager with cache
			mockPI := &mocks.MockPIService{}
			manager, err := NewMetricManager(mockPI, testutils.CreateDefaultParsedTestConfig(), testutils.TestRegion)
			if err != nil {
				return false
			}

			// Pre-populate cache with some metrics
			now := time.Now()
			cachedValues := make(map[string]float64)
			for i := 0; i < numCached; i++ {
				value := float64(i * 50)
				cachedValues[metricNames[i]] = value

				cacheKey := cache.CacheKey{

					Instance:   instance.Identifier,
					MetricName: metricNames[i],
					Statistic:  "avg",
				}
				manager.metricDataCache.Set(cacheKey, value, now, 10*time.Minute)
			}

			// Call filterCachedMetrics
			metricsToFetch, cachedMetrics := manager.filterCachedMetrics(instance, metricNames)

			// Property: Cached metrics should be returned with correct values
			if len(cachedMetrics) != numCached {
				return false
			}

			// Verify cached values match what we stored
			for _, metricDatum := range cachedMetrics {
				expectedValue, exists := cachedValues[metricDatum.Metric]
				if !exists {
					return false
				}
				if metricDatum.Value != expectedValue {
					return false
				}
			}

			// Property: Only non-cached metrics should need fetching
			if len(metricsToFetch) != (numMetrics - numCached) {
				return false
			}

			// Verify that metrics to fetch are not in the cached set
			for _, metricName := range metricsToFetch {
				if _, exists := cachedValues[metricName]; exists {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 15),
		gen.IntRange(0, 15),
	))

	properties.TestingRun(t)
}
