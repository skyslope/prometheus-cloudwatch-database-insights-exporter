package sql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/models"
)

type PostgresClient struct {
	credentials []models.ParsedQueryCredential
	timeout     time.Duration
}

func NewPostgresClient(credentials []models.ParsedQueryCredential) *PostgresClient {
	return &PostgresClient{
		credentials: credentials,
		timeout:     5 * time.Second,
	}
}

// GetTopQueryStats queries pg_stat_statements for top queries by duration and calls.
// Runs two queries and deduplicates by queryid.
func (c *PostgresClient) GetTopQueryStats(ctx context.Context, endpoint string, port int32, clusterIdentifier string, topN int) ([]QueryStats, error) {
	username, password, found := getCredentialsForCluster(c.credentials, clusterIdentifier)
	if !found {
		return nil, fmt.Errorf("no credentials configured for cluster %s", clusterIdentifier)
	}

	// Connect to the default 'postgres' database. pg_stat_statements collects data
	// globally and is readable from any database where the extension is installed.
	timeoutSec := int(c.timeout.Seconds())
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=require&connect_timeout=%d",
		username, password, endpoint, port, timeoutSec)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection to %s: %w", endpoint, err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(10 * time.Second)

	// pg_stat_statements columns:
	//   queryid         - bigint, normalized query fingerprint
	//   query           - text, normalized SQL text (parameters replaced with $1, $2, etc.)
	//   calls           - bigint, number of times executed
	//   mean_exec_time  - double, average execution time in milliseconds
	//   total_exec_time - double, total execution time in milliseconds
	//   rows            - bigint, total rows returned across all executions
	//   blk_read_time   - double, total I/O read time in ms (may be 0 if track_io_timing off)
	//   blk_write_time  - double, total I/O write time in ms
	baseQuery := `
		SELECT
			COALESCE(queryid, 0) AS queryid,
			COALESCE(LEFT(query, 200), '') AS digest_text,
			calls,
			mean_exec_time AS avg_duration_ms,
			COALESCE(blk_read_time, 0) + COALESCE(blk_write_time, 0) AS io_time_ms,
			rows AS rows_returned
		FROM pg_stat_statements
		WHERE queryid IS NOT NULL
		ORDER BY %s DESC
		LIMIT %d`

	seen := make(map[string]bool)
	var results []QueryStats

	for _, orderBy := range []string{"mean_exec_time", "calls"} {
		query := fmt.Sprintf(baseQuery, orderBy, topN)
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			log.Printf("[POSTGRES] Error querying pg_stat_statements on %s (order by %s): %v", endpoint, orderBy, err)
			continue
		}

		for rows.Next() {
			var queryid int64
			var text string
			var calls int64
			var avgMs, ioMs float64
			var rowsReturned int64
			if err := rows.Scan(&queryid, &text, &calls, &avgMs, &ioMs, &rowsReturned); err != nil {
				log.Printf("[POSTGRES] Error scanning row from %s: %v", endpoint, err)
				continue
			}
			digest := strconv.FormatInt(queryid, 10)
			if !seen[digest] {
				seen[digest] = true
				results = append(results, QueryStats{
					Digest:          digest,
					DigestText:      text,
					Calls:           calls,
					AvgDurationMs:   avgMs,
					SumLockTimeMs:   ioMs, // map I/O wait time to "lock time" slot so metric naming stays consistent
					SumRowsExamined: rowsReturned,
					SumRowsSent:     rowsReturned, // pg_stat_statements doesn't distinguish examined vs sent
					SumErrors:       0,             // pg_stat_statements doesn't track errors
				})
			}
		}
		rows.Close()
	}

	return results, nil
}
