package formatting

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/models"
	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/utils"
)

func ConvertToPrometheusMetric(ch chan<- prometheus.Metric, instance models.Instance, metricData models.MetricData, metricPrefix string) error {

	metricName := utils.TrimStatisticFromMetricName(metricData.Metric)
	if metricName == "" {
		return fmt.Errorf("metric name is empty")
	}
	metric, err := safeGetMetricDetails(instance, metricName)
	if err != nil {
		return err
	}

	metricLabels := []string{"identifier", "engine", "unit"}

	engineShortStr := utils.EngineToShortName(instance.Engine)
	prometheusDesc := buildPrometheusDescription(
		buildPrometheusMetricName(metricPrefix, engineShortStr, metricData.Metric),
		metric.Description,
		metricLabels,
	)

	prometheusMetric, err := prometheus.NewConstMetric(
		prometheusDesc,
		prometheus.GaugeValue,
		metricData.Value,
		instance.Identifier,
		string(instance.Engine),
		metric.Unit,
	)
	if err != nil {
		return err
	}

	ch <- prometheus.NewMetricWithTimestamp(metricData.Timestamp, prometheusMetric)
	return nil
}

func safeGetMetricDetails(instance models.Instance, metricName string) (*models.MetricDetails, error) {
	if instance.Metrics == nil {
		return nil, fmt.Errorf("instance.Metrics is nil for instance %s", instance.Identifier)
	}

	if instance.Metrics.MetricsDetails == nil {
		return nil, fmt.Errorf("instance.Metrics.MetricsDetails is nil for instance %s", instance.Identifier)
	}

	metric, exists := instance.Metrics.MetricsDetails[metricName]
	if !exists {
		return nil, fmt.Errorf("metric %s not found for instance %s", metricName, instance.Identifier)
	}

	return &metric, nil
}

func buildPrometheusDescription(metricNameWithStat string, metricDescription string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(
		metricNameWithStat,
		metricDescription,
		labels,
		nil,
	)
}

func ConvertDimensionToPrometheusMetric(ch chan<- prometheus.Metric, instance models.Instance, data models.DimensionMetricData, metricPrefix string) error {
	engineShortStr := utils.EngineToShortName(instance.Engine)

	var metricName string
	var labels []string
	var labelValues []string

	switch data.Group {
	case "db.sql_tokenized":
		metricName = metricPrefix + "_" + engineShortStr + "_top_sql_load_avg"
		labels = []string{"identifier", "engine", "digest", "statement"}
		labelValues = []string{
			instance.Identifier,
			string(instance.Engine),
			data.Dimensions["db.sql_tokenized.id"],
			truncateLabel(data.Dimensions["db.sql_tokenized.statement"], 200),
		}
	case "db.wait_event":
		metricName = metricPrefix + "_" + engineShortStr + "_wait_event_load_avg"
		labels = []string{"identifier", "engine", "wait_event", "wait_type"}
		labelValues = []string{
			instance.Identifier,
			string(instance.Engine),
			data.Dimensions["db.wait_event.name"],
			data.Dimensions["db.wait_event.type"],
		}
	default:
		return fmt.Errorf("unsupported dimension group: %s", data.Group)
	}

	desc := prometheus.NewDesc(metricName, fmt.Sprintf("Top database load by %s", data.Group), labels, nil)
	m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, data.Value, labelValues...)
	if err != nil {
		return err
	}

	ch <- prometheus.NewMetricWithTimestamp(data.Timestamp, m)
	return nil
}

func ConvertQueryStatsToPrometheusMetrics(ch chan<- prometheus.Metric, identifier string, engine string, metricPrefix string, digest string, statement string, calls int64, avgDurationMs float64, lockTimeMs float64, rowsExamined int64, rowsSent int64, errors int64) {
	engineShort := utils.EngineToShortName(models.Engine(engine))
	labels := []string{"identifier", "engine", "digest", "statement"}
	labelValues := []string{identifier, engine, digest, truncateLabel(statement, 200)}

	metrics := []struct {
		name  string
		desc  string
		value float64
	}{
		{metricPrefix + "_" + engineShort + "_query_calls_total", "Total number of times this query has been executed", float64(calls)},
		{metricPrefix + "_" + engineShort + "_query_avg_duration_ms", "Average execution duration in milliseconds", avgDurationMs},
		{metricPrefix + "_" + engineShort + "_query_lock_time_ms", "Total lock time in milliseconds", lockTimeMs},
		{metricPrefix + "_" + engineShort + "_query_rows_examined_total", "Total rows examined", float64(rowsExamined)},
		{metricPrefix + "_" + engineShort + "_query_rows_sent_total", "Total rows sent to client", float64(rowsSent)},
		{metricPrefix + "_" + engineShort + "_query_errors_total", "Total errors", float64(errors)},
	}

	for _, m := range metrics {
		desc := prometheus.NewDesc(m.name, m.desc, labels, nil)
		metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, m.value, labelValues...)
		if err != nil {
			continue
		}
		ch <- metric
	}
}

func truncateLabel(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func buildPrometheusMetricName(metricPrefix string, engineShortStr string, metricWithStatistic string) string {
	if strings.HasPrefix(metricWithStatistic, "db.") {
		metricPrefix = metricPrefix + "_" + engineShortStr
	}
	return metricPrefix + "_" + utils.SnakeCase(metricWithStatistic)
}
