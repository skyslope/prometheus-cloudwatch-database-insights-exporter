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

	query := fmt.Sprintf(`
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
		ORDER BY AVG_TIMER_WAIT DESC
		LIMIT %d`, topN)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query performance_schema on %s: %w", endpoint, err)
	}
	defer rows.Close()

	var results []QueryStats
	for rows.Next() {
		var qs QueryStats
		if err := rows.Scan(&qs.Digest, &qs.DigestText, &qs.Calls, &qs.AvgDurationMs, &qs.SumLockTimeMs, &qs.SumRowsExamined, &qs.SumRowsSent, &qs.SumErrors); err != nil {
			log.Printf("[MYSQL] Error scanning row from %s: %v", endpoint, err)
			continue
		}
		results = append(results, qs)
	}

	return results, nil
}
