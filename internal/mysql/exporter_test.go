package mysql

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestExportReturnsSortedCreateTableDefinitions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(regexp.QuoteMeta("SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'")).
		WillReturnRows(sqlmock.NewRows([]string{"Tables_in_app", "Table_type"}).
			AddRow("zeta", "BASE TABLE").
			AddRow("view_only", "VIEW").
			AddRow("alpha", "BASE TABLE"))
	mock.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE `alpha`")).
		WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).
			AddRow("alpha", "CREATE TABLE `alpha` (\n  `id` bigint NOT NULL\n) ENGINE=InnoDB"))
	mock.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE `zeta`")).
		WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).
			AddRow("zeta", "CREATE TABLE `zeta` (\n  `id` bigint NOT NULL\n) ENGINE=InnoDB;"))

	var buf bytes.Buffer
	if err := Export(context.Background(), db, &buf); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	want := "CREATE TABLE `alpha` (\n  `id` bigint NOT NULL\n) ENGINE=InnoDB;\n\nCREATE TABLE `zeta` (\n  `id` bigint NOT NULL\n) ENGINE=InnoDB;\n"
	if got := buf.String(); got != want {
		t.Fatalf("Export() = %q, want %q", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestExportReturnsEmptyForDatabaseWithoutBaseTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(listBaseTablesQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"Tables_in_app", "Table_type"}).
			AddRow("view_only", "VIEW"))

	var buf bytes.Buffer
	if err := Export(context.Background(), db, &buf); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("Export() = %q, want empty output", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestExportQuotesBackticksInTableNames(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(listBaseTablesQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"Tables_in_app", "Table_type"}).
			AddRow("we`ird", "BASE TABLE"))
	mock.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE `we``ird`")).
		WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).
			AddRow("we`ird", "CREATE TABLE `we``ird` (\n  `id` int\n)"))

	var buf bytes.Buffer
	if err := Export(context.Background(), db, &buf); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if got, want := buf.String(), "CREATE TABLE `we``ird` (\n  `id` int\n);\n"; got != want {
		t.Fatalf("Export() = %q, want quoted identifier", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}
