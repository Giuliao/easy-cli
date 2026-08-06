package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

const listBaseTablesQuery = "SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'"

func Export(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, listBaseTablesQuery)
	if err != nil {
		return "", fmt.Errorf("list base tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var name, tableType string
		if err := rows.Scan(&name, &tableType); err != nil {
			return "", fmt.Errorf("read table list: %w", err)
		}
		if tableType == "BASE TABLE" {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read table list: %w", err)
	}
	sort.Strings(tables)

	ddls := make([]string, 0, len(tables))
	for _, table := range tables {
		var returnedName, ddl string
		query := "SHOW CREATE TABLE " + quoteIdentifier(table)
		if err := db.QueryRowContext(ctx, query).Scan(&returnedName, &ddl); err != nil {
			return "", fmt.Errorf("show create table %q: %w", table, err)
		}
		ddl = strings.TrimSpace(ddl)
		if ddl == "" {
			return "", fmt.Errorf("show create table %q: empty definition", table)
		}
		if !strings.HasSuffix(ddl, ";") {
			ddl += ";"
		}
		ddls = append(ddls, ddl)
	}
	if len(ddls) == 0 {
		return "", nil
	}
	return strings.Join(ddls, "\n\n") + "\n", nil
}

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}
