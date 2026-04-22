package sql

import (
	"context"
	"fmt"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/models"
)

// QueryStats is a unified representation of per-query statistics across database engines.
// Different engines (MySQL performance_schema, Postgres pg_stat_statements, etc.) populate
// this struct from their own catalog views.
type QueryStats struct {
	Digest          string
	DigestText      string
	Calls           int64
	AvgDurationMs   float64
	SumLockTimeMs   float64
	SumRowsExamined int64
	SumRowsSent     int64
	SumErrors       int64
}

// EngineClient is the common interface for per-engine query stats collectors.
type EngineClient interface {
	GetTopQueryStats(ctx context.Context, endpoint string, port int32, topN int) ([]QueryStats, error)
}

// Client is the main dispatcher that routes requests to the appropriate engine client
// based on the database engine.
type Client struct {
	credentials []models.ParsedQueryCredential
	mysql       *MySQLClient
	postgres    *PostgresClient
}

func NewClient(credentials []models.ParsedQueryCredential) *Client {
	return &Client{
		credentials: credentials,
		mysql:       NewMySQLClient(credentials),
		postgres:    NewPostgresClient(credentials),
	}
}

// IsConfigured returns true if any credentials are configured.
func (c *Client) IsConfigured() bool {
	return len(c.credentials) > 0
}

// GetTopQueryStats routes to the appropriate engine-specific client based on instance.Engine.
// Returns (nil, nil) silently when no credentials are configured for the cluster (expected
// state - means we don't want to collect query metrics for that cluster) or when the engine
// is unsupported.
func (c *Client) GetTopQueryStats(ctx context.Context, instance models.Instance, topN int) ([]QueryStats, error) {
	// Silently skip if no credentials match this cluster
	if _, _, found := getCredentialsForCluster(c.credentials, instance.ClusterIdentifier); !found {
		return nil, nil
	}

	switch instance.Engine {
	case models.MySQL, models.AuroraMySQL, models.MariaDB:
		return c.mysql.GetTopQueryStats(ctx, instance.Endpoint, instance.Port, instance.ClusterIdentifier, topN)
	case models.PostgreSQL, models.AuroraPostgreSQL:
		return c.postgres.GetTopQueryStats(ctx, instance.Endpoint, instance.Port, instance.ClusterIdentifier, topN)
	default:
		return nil, fmt.Errorf("unsupported engine for query metrics: %s", instance.Engine)
	}
}

// GetCredentialsForCluster looks up a credential entry by cluster name, falling back
// to the default (empty cluster name) if no specific match is found.
func getCredentialsForCluster(credentials []models.ParsedQueryCredential, clusterIdentifier string) (string, string, bool) {
	var defaultCred *models.ParsedQueryCredential
	for i := range credentials {
		if credentials[i].Cluster == clusterIdentifier {
			return credentials[i].Username, credentials[i].Password, true
		}
		if credentials[i].Cluster == "" {
			defaultCred = &credentials[i]
		}
	}
	if defaultCred != nil {
		return defaultCred.Username, defaultCred.Password, true
	}
	return "", "", false
}
