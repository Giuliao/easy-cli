package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bytedance/easy-cli/internal/config"
	"github.com/bytedance/easy-cli/internal/mysql"
	"github.com/bytedance/easy-cli/internal/skill"
)

func TestRunMySQLDDLPassesConnectionOptionsToExporter(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var got mysql.ConnectionOptions
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "ddl",
		"--host", "db.internal",
		"--port", "3307",
		"--user", "app-user",
		"--password", "secret",
		"--database", "orders",
	}, registry, Options{
		Out:    &stdout,
		ErrOut: &stderr,
		MySQLExport: func(_ context.Context, options mysql.ConnectionOptions, out io.Writer) error {
			got = options
			_, _ = io.WriteString(out, "CREATE TABLE `orders` (\n  `id` bigint NOT NULL\n);\n")
			return nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got.Host != "db.internal" || got.Port != 3307 || got.User != "app-user" || got.Password != "secret" || got.Database != "orders" {
		t.Fatalf("connection options = %+v", got)
	}
	if stdout.String() != "CREATE TABLE `orders` (\n  `id` bigint NOT NULL\n);\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunMySQLDDLReadsPasswordFromStdin(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var password string
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "ddl", "--host", "db.internal", "--user", "app-user",
		"--password-stdin", "--database", "orders",
	}, registry, Options{
		In:  strings.NewReader("secret with spaces\n"),
		Out: &stdout, ErrOut: &stderr,
		MySQLExport: func(_ context.Context, options mysql.ConnectionOptions, _ io.Writer) error {
			password = options.Password
			return nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if password != "secret with spaces" {
		t.Fatalf("password = %q, want stdin value", password)
	}
	if strings.Contains(stdout.String()+stderr.String(), password) {
		t.Fatal("password leaked to command output")
	}
}

func TestRunMySQLDDLUsesConfiguredConnection(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var got mysql.ConnectionOptions
	var stdout, stderr bytes.Buffer
	code := Run([]string{"mysql", "ddl"}, registry, Options{
		Config: config.Config{MySQL: config.MySQL{
			Host: "configured.db.internal", Port: 3307, User: "configured-user", Password: "configured-password", Database: "configured-db",
		}},
		Out: &stdout, ErrOut: &stderr,
		MySQLExport: func(_ context.Context, options mysql.ConnectionOptions, _ io.Writer) error {
			got = options
			return nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got.Host != "configured.db.internal" || got.Port != 3307 || got.User != "configured-user" || got.Password != "configured-password" || got.Database != "configured-db" {
		t.Fatalf("connection options = %+v, want configured values", got)
	}
}

func TestRunMySQLDDLFlagsOverrideConfiguredConnection(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var got mysql.ConnectionOptions
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "ddl", "--host", "flag.db.internal", "--port", "3310", "--user", "flag-user", "--password", "flag-password", "--database", "flag-db",
	}, registry, Options{
		Config: config.Config{MySQL: config.MySQL{
			Host: "configured.db.internal", Port: 3307, User: "configured-user", Password: "configured-password", Database: "configured-db",
		}},
		Out: &stdout, ErrOut: &stderr,
		MySQLExport: func(_ context.Context, options mysql.ConnectionOptions, _ io.Writer) error {
			got = options
			return nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got.Host != "flag.db.internal" || got.Port != 3310 || got.User != "flag-user" || got.Password != "flag-password" || got.Database != "flag-db" {
		t.Fatalf("connection options = %+v, want flag values", got)
	}
}

func TestRunMySQLQueryStdinPasswordOverridesConfiguredPassword(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var password string
	var stdout, stderr bytes.Buffer
	code := Run([]string{"mysql", "query", "--password-stdin", "--sql", "SELECT 1"}, registry, Options{
		Config: config.Config{MySQL: config.MySQL{
			Host: "configured.db.internal", Port: 3307, User: "configured-user", Password: "configured-password", Database: "configured-db",
		}},
		In: strings.NewReader("stdin-password\n"), Out: &stdout, ErrOut: &stderr,
		MySQLQuery: func(_ context.Context, options mysql.ConnectionOptions, _ string) (mysql.QueryResult, error) {
			password = options.Password
			return mysql.QueryResult{}, nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if password != "stdin-password" {
		t.Fatalf("password = %q, want stdin password", password)
	}
	if strings.Contains(stdout.String()+stderr.String(), "stdin-password") {
		t.Fatal("stdin password leaked to command output")
	}
}

func TestRunMySQLDDLRejectsConflictingPasswordFlags(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "ddl", "--host", "db.internal", "--user", "app-user",
		"--password", "secret", "--password-stdin", "--database", "orders",
	}, registry, Options{
		Out: &stdout, ErrOut: &stderr,
		MySQLExport: func(context.Context, mysql.ConnectionOptions, io.Writer) error {
			called = true
			return nil
		},
	})

	if code != 2 {
		t.Fatalf("Run() code = %d, want 2", code)
	}
	if called {
		t.Fatal("exporter called for invalid password flags")
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Fatalf("stderr = %q, want mutually exclusive error", stderr.String())
	}
}

func TestRunMySQLDDLDoesNotLeakPasswordWhenExportFails(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "ddl", "--host", "db.internal", "--user", "app-user",
		"--password", "secret", "--database", "orders",
	}, registry, Options{
		Out: &stdout, ErrOut: &stderr,
		MySQLExport: func(context.Context, mysql.ConnectionOptions, io.Writer) error {
			return errors.New("access denied")
		},
	})

	if code != 1 {
		t.Fatalf("Run() code = %d, want 1", code)
	}
	if strings.Contains(stdout.String()+stderr.String(), "secret") {
		t.Fatal("password leaked when export failed")
	}
}

func TestRunMySQLQueryPassesStatementAndOutputsJSON(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var got mysql.ConnectionOptions
	var gotStatement string
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "query",
		"--host", "db.internal",
		"--port", "3307",
		"--user", "app-user",
		"--password", "secret",
		"--database", "orders",
		"--sql", "DELETE FROM orders",
	}, registry, Options{
		Out:    &stdout,
		ErrOut: &stderr,
		MySQLQuery: func(_ context.Context, options mysql.ConnectionOptions, statement string) (mysql.QueryResult, error) {
			got = options
			gotStatement = statement
			return mysql.QueryResult{Columns: []string{"id", "name"}, Rows: [][]any{{int64(1), "alice"}}}, nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got.Host != "db.internal" || got.Port != 3307 || got.User != "app-user" || got.Password != "secret" || got.Database != "orders" {
		t.Fatalf("connection options = %+v", got)
	}
	if gotStatement != "DELETE FROM orders" {
		t.Fatalf("statement = %q, want arbitrary SQL passed through", gotStatement)
	}
	if stdout.String() != "{\"columns\":[\"id\",\"name\"],\"rows\":[[1,\"alice\"]]}\n" {
		t.Fatalf("stdout = %q, want JSON result", stdout.String())
	}
}

func TestRunMySQLQueryOutputsTableFormat(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "query",
		"--host", "db.internal", "--user", "app-user",
		"--password", "secret", "--database", "orders",
		"--sql", "SELECT id, name FROM orders", "--format", "table",
	}, registry, Options{
		Out: &stdout, ErrOut: &stderr,
		MySQLQuery: func(_ context.Context, _ mysql.ConnectionOptions, _ string) (mysql.QueryResult, error) {
			return mysql.QueryResult{Columns: []string{"id", "name"}, Rows: [][]any{{int64(1), "alice"}, {nil, "bob"}}}, nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "id\tname\n1\talice\n\tbob\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want table output %q", stdout.String(), want)
	}
}

func TestRunMySQLQueryReadsPasswordFromStdinWithoutLeakingIt(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var password string
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "query", "--host", "db.internal", "--user", "app-user",
		"--password-stdin", "--database", "orders", "--sql", "SELECT 1",
	}, registry, Options{
		In: strings.NewReader("secret with spaces\n"), Out: &stdout, ErrOut: &stderr,
		MySQLQuery: func(_ context.Context, options mysql.ConnectionOptions, _ string) (mysql.QueryResult, error) {
			password = options.Password
			return mysql.QueryResult{}, nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if password != "secret with spaces" {
		t.Fatalf("password = %q, want stdin value", password)
	}
	if strings.Contains(stdout.String()+stderr.String(), password) {
		t.Fatal("password leaked to command output")
	}
}

func TestRunMySQLQueryRejectsInvalidFormat(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"mysql", "query", "--host", "db.internal", "--user", "app-user",
		"--password", "secret", "--database", "orders", "--sql", "SELECT 1",
		"--format", "xml",
	}, registry, Options{
		Out: &stdout, ErrOut: &stderr,
		MySQLQuery: func(context.Context, mysql.ConnectionOptions, string) (mysql.QueryResult, error) {
			called = true
			return mysql.QueryResult{}, nil
		},
	})

	if code != 2 {
		t.Fatalf("Run() code = %d, want 2", code)
	}
	if called {
		t.Fatal("query function called for invalid format")
	}
	if !strings.Contains(stderr.String(), "--format must be json or table") {
		t.Fatalf("stderr = %q, want format error", stderr.String())
	}
}

func TestRunMySQLQueryHelpDescribesUnrestrictedSQL(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"mysql", "query", "--help"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--sql <statement>") || !strings.Contains(stdout.String(), "without CLI filtering") {
		t.Fatalf("stdout = %q, want SQL behavior description", stdout.String())
	}
}

func TestRunMySQLQueryDoesNotTreatSQLValueAsHelp(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"mysql", "query", "--sql", "--help"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 2 {
		t.Fatalf("Run() code = %d, want 2; stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--sql requires a non-empty value") {
		t.Fatalf("stderr = %q, want SQL value error", stderr.String())
	}
}

func TestRunMySQLQueryAllowsStatementStartingWithComment(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	statement := "-- explain this query\nSELECT 1"
	var gotStatement string
	var stdout, stderr bytes.Buffer
	code := Run([]string{"mysql", "query", "--sql", statement}, registry, Options{
		Config: config.Config{MySQL: config.MySQL{
			Host: "configured.db.internal", Port: 3306, User: "configured-user", Password: "configured-password", Database: "configured-db",
		}},
		Out: &stdout, ErrOut: &stderr,
		MySQLQuery: func(_ context.Context, _ mysql.ConnectionOptions, sql string) (mysql.QueryResult, error) {
			gotStatement = sql
			return mysql.QueryResult{}, nil
		},
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if gotStatement != statement {
		t.Fatalf("statement = %q, want %q", gotStatement, statement)
	}
}
