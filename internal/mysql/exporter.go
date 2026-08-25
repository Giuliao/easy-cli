package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const listBaseTablesQuery = "SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'"
const maxDDLConcurrency = 8

func Export(ctx context.Context, db *sql.DB, out io.Writer) error {
	tables, err := listBaseTables(ctx, db)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}

	type tableResult struct {
		index int
		ddl   string
		err   error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan tableResult, len(tables))
	sem := make(chan struct{}, maxDDLConcurrency)

	go func() {
		var wg sync.WaitGroup
		for i, table := range tables {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func(i int, table string) {
				defer wg.Done()
				defer func() { <-sem }()
				ddl, err := showCreateTable(ctx, db, table)
				results <- tableResult{index: i, ddl: ddl, err: err}
			}(i, table)
		}
		wg.Wait()
		close(results)
	}()

	buffer := make(map[int]string)
	next := 0
	for r := range results {
		if r.err != nil {
			cancel()
			return r.err
		}
		buffer[r.index] = r.ddl
		for {
			ddl, ok := buffer[next]
			if !ok {
				break
			}
			delete(buffer, next)
			if _, err := io.WriteString(out, ddl); err != nil {
				cancel()
				return err
			}
			if next < len(tables)-1 {
				if _, err := io.WriteString(out, "\n\n"); err != nil {
					cancel()
					return err
				}
			} else {
				if _, err := io.WriteString(out, "\n"); err != nil {
					cancel()
					return err
				}
			}
			next++
		}
	}
	return nil
}

func listBaseTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, listBaseTablesQuery)
	if err != nil {
		return nil, fmt.Errorf("list base tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var name, tableType string
		if err := rows.Scan(&name, &tableType); err != nil {
			return nil, fmt.Errorf("read table list: %w", err)
		}
		if tableType == "BASE TABLE" {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table list: %w", err)
	}
	sort.Strings(tables)
	return tables, nil
}

func showCreateTable(ctx context.Context, db *sql.DB, table string) (string, error) {
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
	return ddl, nil
}

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}
