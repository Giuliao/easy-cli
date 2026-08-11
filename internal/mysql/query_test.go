package mysql

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestQueryRowsReturnsColumnsAndRowsInDatabaseOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statement := "SELECT id, name FROM users"
	mock.ExpectQuery(regexp.QuoteMeta(statement)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(int64(2), "bob").
			AddRow(int64(1), "alice"))

	got, err := QueryRows(context.Background(), db, statement)
	if err != nil {
		t.Fatalf("QueryRows() error = %v", err)
	}
	want := QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(2), "bob"}, {int64(1), "alice"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRows() = %#v, want %#v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestQueryRowsConvertsNullTimeAndBytes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statement := "SELECT value, created_at, payload"
	createdAt := time.Date(2026, 8, 11, 12, 13, 14, 123000000, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(statement)).
		WillReturnRows(sqlmock.NewRows([]string{"value", "created_at", "payload"}).
			AddRow([]byte("hello"), createdAt, []byte{0xff, 0x00}))

	got, err := QueryRows(context.Background(), db, statement)
	if err != nil {
		t.Fatalf("QueryRows() error = %v", err)
	}
	want := QueryResult{
		Columns: []string{"value", "created_at", "payload"},
		Rows: [][]any{{
			"hello",
			"2026-08-11T12:13:14.123Z",
			"base64:/wA=",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRows() = %#v, want %#v", got, want)
	}
}

func TestQueryRowsReturnsEmptyRowsWithColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statement := "SELECT id FROM users WHERE false"
	mock.ExpectQuery(regexp.QuoteMeta(statement)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	got, err := QueryRows(context.Background(), db, statement)
	if err != nil {
		t.Fatalf("QueryRows() error = %v", err)
	}
	want := QueryResult{Columns: []string{"id"}, Rows: [][]any{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRows() = %#v, want %#v", got, want)
	}
}

func TestQueryRowsReturnsQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statement := "DELETE FROM users"
	mock.ExpectQuery(regexp.QuoteMeta(statement)).WillReturnError(errors.New("permission denied"))

	_, err = QueryRows(context.Background(), db, statement)
	if err == nil || err.Error() != "query mysql: permission denied" {
		t.Fatalf("QueryRows() error = %v, want wrapped query error", err)
	}
}
