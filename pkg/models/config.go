package models

import (
	"time"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/filter"
)

type Config struct {
	Discovery DiscoveryConfig
	Export    ExportConfig
}

type DiscoveryConfig struct {
	Regions      []string
	Instances    InstancesConfig
	Metrics      MetricsConfig
	Dimensions   DimensionsConfig   `yaml:"dimensions,omitempty"`
	QueryMetrics QueryMetricsConfig `yaml:"query-metrics,omitempty"`
	Processing   ProcessingConfig
}

type DimensionsConfig struct {
	Enabled bool     `yaml:"enabled"`
	TopN    int      `yaml:"top-n"`
	Groups  []string `yaml:"groups"`
}

type QueryMetricsConfig struct {
	Enabled     bool                    `yaml:"enabled"`
	TopN        int                     `yaml:"top-n"`
	Credentials []QueryCredentialConfig `yaml:"credentials,omitempty"`
}

type QueryCredentialConfig struct {
	Cluster     string `yaml:"cluster"`
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password-env"`
}

type ExportConfig struct {
	Port       int
	Prometheus PrometheusConfig
}

type InstancesConfig struct {
	MaxInstances int                `yaml:"max-instances"`
	Cache        InstancesCacheConfig `yaml:"cache,omitempty"`
	Include      FilterConfig       `yaml:"include,omitempty"`
	Exclude      FilterConfig       `yaml:"exclude,omitempty"`
}

type InstancesCacheConfig struct {
	TTL string `yaml:"ttl"`
}

type MetricsConfig struct {
	Statistic string
	Cache     MetricsCacheConfig `yaml:"cache,omitempty"`
	Include   FilterConfig       `yaml:"include,omitempty"`
	Exclude   FilterConfig       `yaml:"exclude,omitempty"`
}

type MetricsCacheConfig struct {
	MetricMetadataTTL string                `yaml:"metric-metadata-ttl"`
	MetricData        MetricDataCacheConfig `yaml:"metric-data"`
}

type ProcessingConfig struct {
	Concurrency int
}

type PrometheusConfig struct {
	MetricPrefix string `yaml:"metric-prefix"`
}

type MetricDataCacheConfig struct {
	DefaultTTL  string             `yaml:"default-ttl,omitempty"`
	PatternTTLs []PatternTTLConfig `yaml:"pattern-ttls,omitempty"`
	MaxSize     int                `yaml:"max-size"`
}

type PatternTTLConfig struct {
	Pattern string `yaml:"pattern"`
	TTL     string `yaml:"ttl"`
}

type FilterConfig map[string][]string

type ParsedConfig struct {
	Discovery ParsedDiscoveryConfig
	Export    ParsedExportConfig
}

type ParsedDiscoveryConfig struct {
	Regions      []string
	Instances    ParsedInstancesConfig
	Metrics      ParsedMetricsConfig
	Dimensions   ParsedDimensionsConfig
	QueryMetrics ParsedQueryMetricsConfig
	Processing   ParsedProcessingConfig
}

type ParsedDimensionsConfig struct {
	Enabled bool
	TopN    int32
	Groups  []string
}

type ParsedQueryMetricsConfig struct {
	Enabled     bool
	TopN        int
	Credentials []ParsedQueryCredential
}

type ParsedQueryCredential struct {
	Cluster  string
	Username string
	Password string
}

type ParsedExportConfig struct {
	Port       int
	Prometheus ParsedPrometheusConfig
}

type ParsedInstancesConfig struct {
	MaxInstances int `yaml:"max-instances"`
	CacheTTL     time.Duration
	Filter       filter.Filter
}

type ParsedMetricsConfig struct {
	Statistic         Statistic
	MetadataCacheTTL  time.Duration
	DataCacheMaxSize  int
	DataCachePatterns []ParsedPatternTTL
	Filter            filter.Filter
	Include           FilterConfig
	Exclude           FilterConfig
}

type ParsedProcessingConfig struct {
	Concurrency int
}

type ParsedPrometheusConfig struct {
	MetricPrefix string `yaml:"metric-prefix"`
}

type ParsedPatternTTL struct {
	Pattern string
	TTL     time.Duration
}

func (instanceConfig *ParsedInstancesConfig) ShouldIncludeInstance(instance filter.Filterable) bool {
	if instanceConfig.Filter == nil {
		return true
	}
	return instanceConfig.Filter.ShouldInclude(instance)
}

func (metricConfig *ParsedMetricsConfig) ShouldIncludeMetric(metricDetails filter.Filterable) bool {
	if metricConfig.Filter == nil {
		return true
	}
	return metricConfig.Filter.ShouldInclude(metricDetails)
}
