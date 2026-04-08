package pi

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/pi"
)

type PIService interface {
	ListAvailableResourceMetrics(ctx context.Context, resourceID string) (*pi.ListAvailableResourceMetricsOutput, error)
	GetResourceMetrics(ctx context.Context, resourceID string, metricNames []string) (*pi.GetResourceMetricsOutput, error)
	GetResourceMetricsWithDimensions(ctx context.Context, resourceID string, metric string, dimensionGroup string, limit int32) (*pi.GetResourceMetricsOutput, error)
}
