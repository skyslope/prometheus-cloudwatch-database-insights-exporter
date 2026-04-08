package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type QueryStats struct {
	Digest           string
	DigestText       string
	Calls            int64
	AvgDurationMs    float64
	SumLockTimeMs    float64
	SumRowsExamined  int64
	SumRowsSent      int64
	SumErrors        int64
}

type MySQLClient struct {
	username string
	password string
	timeout  time.Duration
}

func NewMySQLClient() *MySQLClient {
	username := os.Getenv("DB_USERNAME")
	password := os.Getenv("DB_PASSWORD")
	if username == "" {
		username = "dbi_reader"
	}
	return &MySQLClient{
		username: username,
		password: password,
		timeout:  5 * time.Second,
	}
}

func (c *MySQLClient) IsConfigured() bool {
	return c.password != ""
}

func (c *MySQLClient) GetTopQueryStats(ctx context.Context, endpoint string, port int32, topN int) ([]QueryStats, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/performance_schema?timeout=%s&readTimeout=%s",
		c.username, c.password, endpoint, port, c.timeout, c.timeout)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection to %s: %w", endpoint, err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(10 * time.Second)

	baseQuery := `
		SELECT
			COALESCE(DIGEST, '') AS digest,
			COALESCE(LEFT(DIGEST_TEXT, 200), '') AS digest_text,
			COUNT_STAR AS calls,
			(AVG_TIMER_WAIT / 1000000000) AS avg_duration_ms,
			(SUM_LOCK_TIME / 1000000000000) AS sum_lock_time_ms,
			SUM_ROWS_EXAMINED AS rows_examined,
			SUM_ROWS_SENT AS rows_sent,
			SUM_ERRORS AS errors
		FROM events_statements_summary_by_digest
		WHERE DIGEST IS NOT NULL AND SCHEMA_NAME IS NOT NULL
		ORDER BY %s DESC
		LIMIT %d`

	seen := make(map[string]bool)
	var results []QueryStats

	// Run two queries: top by duration, then top by calls
	for _, orderBy := range []string{"AVG_TIMER_WAIT", "COUNT_STAR"} {
		query := fmt.Sprintf(baseQuery, orderBy, topN)
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			log.Printf("[MYSQL] Error querying performance_schema on %s (order by %s): %v", endpoint, orderBy, err)
			continue
		}

		for rows.Next() {
			var qs QueryStats
			if err := rows.Scan(&qs.Digest, &qs.DigestText, &qs.Calls, &qs.AvgDurationMs, &qs.SumLockTimeMs, &qs.SumRowsExamined, &qs.SumRowsSent, &qs.SumErrors); err != nil {
				log.Printf("[MYSQL] Error scanning row from %s: %v", endpoint, err)
				continue
			}
			if !seen[qs.Digest] {
				seen[qs.Digest] = true
				results = append(results, qs)
			}
		}
		rows.Close()
	}

	return results, nil
}
