package mysql

import (
	"testing"

	driver "github.com/go-sql-driver/mysql"
)

func TestBuildDSNUsesConfiguredConnectionValues(t *testing.T) {
	got, err := buildDSN(ConnectionOptions{
		Host:     "db.internal",
		Port:     3307,
		User:     "app-user",
		Password: "secret",
		Database: "orders",
	})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}
	parsed, err := driver.ParseDSN(got)
	if err != nil {
		t.Fatalf("ParseDSN() error = %v", err)
	}
	if parsed.Net != "tcp" || parsed.Addr != "db.internal:3307" || parsed.User != "app-user" || parsed.Passwd != "secret" || parsed.DBName != "orders" {
		t.Fatalf("parsed DSN = net=%q addr=%q user=%q db=%q", parsed.Net, parsed.Addr, parsed.User, parsed.DBName)
	}
}
