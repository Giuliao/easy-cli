package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"strconv"

	driver "github.com/go-sql-driver/mysql"
)

type ConnectionOptions struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func Open(ctx context.Context, options ConnectionOptions) (*sql.DB, error) {
	dsn, err := buildDSN(options)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql connection: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}

func ExportDDL(ctx context.Context, options ConnectionOptions, out io.Writer) error {
	db, err := Open(ctx, options)
	if err != nil {
		return err
	}
	defer db.Close()
	return Export(ctx, db, out)
}

func buildDSN(options ConnectionOptions) (string, error) {
	if options.Host == "" {
		return "", fmt.Errorf("mysql host is required")
	}
	if options.Port < 1 || options.Port > 65535 {
		return "", fmt.Errorf("mysql port must be between 1 and 65535")
	}
	if options.User == "" {
		return "", fmt.Errorf("mysql user is required")
	}
	if options.Database == "" {
		return "", fmt.Errorf("mysql database is required")
	}
	cfg := driver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(options.Host, strconv.Itoa(options.Port))
	cfg.User = options.User
	cfg.Passwd = options.Password
	cfg.DBName = options.Database
	cfg.ParseTime = true
	return cfg.FormatDSN(), nil
}
