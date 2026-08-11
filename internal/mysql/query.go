package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"
	"unicode/utf8"
)

type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

func Query(ctx context.Context, options ConnectionOptions, statement string) (QueryResult, error) {
	db, err := Open(ctx, options)
	if err != nil {
		return QueryResult{}, err
	}
	defer db.Close()
	return QueryRows(ctx, db, statement)
}

func QueryRows(ctx context.Context, db *sql.DB, statement string) (QueryResult, error) {
	rows, err := db.QueryContext(ctx, statement)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query mysql: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return QueryResult{}, fmt.Errorf("read query columns: %w", err)
	}
	result := QueryResult{
		Columns: columns,
		Rows:    make([][]any, 0),
	}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return QueryResult{}, fmt.Errorf("read query row: %w", err)
		}
		for i := range values {
			values[i] = normalizeQueryValue(values[i])
		}
		result.Rows = append(result.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, fmt.Errorf("read query rows: %w", err)
	}
	return result, nil
}

func normalizeQueryValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		if utf8.Valid(typed) {
			return string(typed)
		}
		return "base64:" + base64.StdEncoding.EncodeToString(typed)
	case sql.RawBytes:
		if utf8.Valid(typed) {
			return string(typed)
		}
		return "base64:" + base64.StdEncoding.EncodeToString(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return value
	}
}
