package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/easy-cli/internal/config"
	"github.com/bytedance/easy-cli/internal/mysql"
)

const defaultMySQLPort = 3306
const mySQLExportTimeout = 30 * time.Second

type mysqlConnectionOverrides struct {
	Host          string
	HostSet       bool
	Port          int
	PortSet       bool
	User          string
	UserSet       bool
	Password      string
	PasswordSet   bool
	PasswordStdin bool
	Database      string
	DatabaseSet   bool
}

func runMySQLDDL(args []string, options Options, out, errOut io.Writer) int {
	if requestsMySQLHelp(args) {
		printMySQLDDLHelp(out)
		return 0
	}
	if options.ConfigErr != nil {
		fmt.Fprintf(errOut, "mysql ddl: load configuration: %v\n", options.ConfigErr)
		return 1
	}
	connection, passwordStdin, _, err := parseMySQLDDLArgs(args, options.Config.MySQL)
	if err != nil {
		fmt.Fprintf(errOut, "mysql ddl: %v\n", err)
		return 2
	}
	if passwordStdin {
		input := options.In
		if input == nil {
			input = os.Stdin
		}
		connection.Password, err = readPassword(input)
		if err != nil {
			fmt.Fprintf(errOut, "mysql ddl: read password: %v\n", err)
			return 1
		}
	}
	exporter := options.MySQLExport
	if exporter == nil {
		exporter = mysql.ExportDDL
	}
	ctx, cancel := context.WithTimeout(context.Background(), mySQLExportTimeout)
	defer cancel()
	ddl, err := exporter(ctx, connection)
	if err != nil {
		fmt.Fprintf(errOut, "mysql ddl: export failed: %v\n", err)
		return 1
	}
	_, _ = io.WriteString(out, ddl)
	return 0
}

func runMySQLQuery(args []string, options Options, out, errOut io.Writer) int {
	if requestsMySQLHelp(args) {
		printMySQLQueryHelp(out)
		return 0
	}
	if options.ConfigErr != nil {
		fmt.Fprintf(errOut, "mysql query: load configuration: %v\n", options.ConfigErr)
		return 1
	}
	connection, passwordStdin, statement, format, _, err := parseMySQLQueryArgs(args, options.Config.MySQL)
	if err != nil {
		fmt.Fprintf(errOut, "mysql query: %v\n", err)
		return 2
	}
	if passwordStdin {
		input := options.In
		if input == nil {
			input = os.Stdin
		}
		connection.Password, err = readPassword(input)
		if err != nil {
			fmt.Fprintf(errOut, "mysql query: read password: %v\n", err)
			return 1
		}
	}
	query := options.MySQLQuery
	if query == nil {
		query = mysql.Query
	}
	ctx, cancel := context.WithTimeout(context.Background(), mySQLExportTimeout)
	defer cancel()
	result, err := query(ctx, connection, statement)
	if err != nil {
		fmt.Fprintf(errOut, "mysql query: execution failed: %v\n", err)
		return 1
	}
	if err := writeMySQLQueryResult(out, result, format); err != nil {
		fmt.Fprintf(errOut, "mysql query: format result: %v\n", err)
		return 1
	}
	return 0
}

func parseMySQLDDLArgs(args []string, configured config.MySQL) (mysql.ConnectionOptions, bool, bool, error) {
	var overrides mysqlConnectionOverrides
	for i := 0; i < len(args); i++ {
		if args[i] == "--help" || args[i] == "-h" {
			return mysql.ConnectionOptions{}, false, true, nil
		}
		matched, next, err := parseMySQLConnectionOption(args, i, &overrides)
		if err != nil {
			return mysql.ConnectionOptions{}, false, false, err
		}
		if !matched {
			return mysql.ConnectionOptions{}, false, false, fmt.Errorf("unknown option %q", args[i])
		}
		i = next
	}
	connection, err := resolveMySQLConnection(configured, overrides)
	if err != nil {
		return mysql.ConnectionOptions{}, false, false, err
	}
	return connection, overrides.PasswordStdin, false, nil
}

func parseMySQLQueryArgs(args []string, configured config.MySQL) (mysql.ConnectionOptions, bool, string, string, bool, error) {
	var overrides mysqlConnectionOverrides
	statement := ""
	statementSet := false
	format := "json"
	for i := 0; i < len(args); i++ {
		if args[i] == "--help" || args[i] == "-h" {
			return mysql.ConnectionOptions{}, false, "", "", true, nil
		}
		matched, next, err := parseMySQLConnectionOption(args, i, &overrides)
		if err != nil {
			return mysql.ConnectionOptions{}, false, "", "", false, err
		}
		if matched {
			i = next
			continue
		}
		switch args[i] {
		case "--sql":
			if i+1 >= len(args) || args[i+1] == "" || args[i+1] == "--format" || args[i+1] == "--help" || args[i+1] == "-h" {
				return mysql.ConnectionOptions{}, false, "", "", false, fmt.Errorf("--sql requires a non-empty value")
			}
			statement, statementSet, i = args[i+1], true, i+1
		case "--format":
			value, next, err := requiredFlagValue(args, i, "--format")
			if err != nil {
				return mysql.ConnectionOptions{}, false, "", "", false, err
			}
			if value != "json" && value != "table" {
				return mysql.ConnectionOptions{}, false, "", "", false, fmt.Errorf("--format must be json or table")
			}
			format, i = value, next
		default:
			return mysql.ConnectionOptions{}, false, "", "", false, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if !statementSet {
		return mysql.ConnectionOptions{}, false, "", "", false, fmt.Errorf("--sql is required")
	}
	connection, err := resolveMySQLConnection(configured, overrides)
	if err != nil {
		return mysql.ConnectionOptions{}, false, "", "", false, err
	}
	return connection, overrides.PasswordStdin, statement, format, false, nil
}

func parseMySQLConnectionOption(args []string, index int, overrides *mysqlConnectionOverrides) (bool, int, error) {
	switch args[index] {
	case "--host":
		value, next, err := requiredFlagValue(args, index, "--host")
		if err != nil {
			return true, index, err
		}
		overrides.Host, overrides.HostSet = value, true
		return true, next, nil
	case "--port":
		value, next, err := requiredFlagValue(args, index, "--port")
		if err != nil {
			return true, index, err
		}
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return true, index, fmt.Errorf("--port must be between 1 and 65535")
		}
		overrides.Port, overrides.PortSet = port, true
		return true, next, nil
	case "--user":
		value, next, err := requiredFlagValue(args, index, "--user")
		if err != nil {
			return true, index, err
		}
		overrides.User, overrides.UserSet = value, true
		return true, next, nil
	case "--password":
		if overrides.PasswordStdin {
			return true, index, fmt.Errorf("--password and --password-stdin are mutually exclusive")
		}
		value, next, err := requiredFlagValue(args, index, "--password")
		if err != nil {
			return true, index, err
		}
		overrides.Password, overrides.PasswordSet = value, true
		return true, next, nil
	case "--password-stdin":
		if overrides.PasswordSet {
			return true, index, fmt.Errorf("--password and --password-stdin are mutually exclusive")
		}
		overrides.PasswordStdin = true
		return true, index, nil
	case "--database":
		value, next, err := requiredFlagValue(args, index, "--database")
		if err != nil {
			return true, index, err
		}
		overrides.Database, overrides.DatabaseSet = value, true
		return true, next, nil
	default:
		return false, index, nil
	}
}

func resolveMySQLConnection(configured config.MySQL, overrides mysqlConnectionOverrides) (mysql.ConnectionOptions, error) {
	connection := mysql.ConnectionOptions{
		Host:     configured.Host,
		Port:     configured.Port,
		User:     configured.User,
		Password: configured.Password,
		Database: configured.Database,
	}
	if overrides.HostSet {
		connection.Host = overrides.Host
	}
	if overrides.PortSet {
		connection.Port = overrides.Port
	}
	if overrides.UserSet {
		connection.User = overrides.User
	}
	if overrides.PasswordSet {
		connection.Password = overrides.Password
	}
	if overrides.DatabaseSet {
		connection.Database = overrides.Database
	}
	if connection.Port == 0 {
		connection.Port = defaultMySQLPort
	}
	if connection.Host == "" {
		return mysql.ConnectionOptions{}, fmt.Errorf("--host is required; set --host or mysql.host in configuration")
	}
	if connection.User == "" {
		return mysql.ConnectionOptions{}, fmt.Errorf("--user is required; set --user or mysql.user in configuration")
	}
	if connection.Database == "" {
		return mysql.ConnectionOptions{}, fmt.Errorf("--database is required; set --database or mysql.database in configuration")
	}
	if connection.Password == "" && !overrides.PasswordStdin {
		return mysql.ConnectionOptions{}, fmt.Errorf("one of --password or --password-stdin is required; set mysql.password in configuration")
	}
	return connection, nil
}

func requestsMySQLHelp(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return true
		case "--host", "--port", "--user", "--password", "--database", "--sql", "--format":
			if i+1 >= len(args) {
				return false
			}
			i++
		case "--password-stdin":
		default:
			return false
		}
	}
	return false
}

func requiredFlagValue(args []string, index int, flagName string) (string, int, error) {
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
		return "", index, fmt.Errorf("%s requires a value", flagName)
	}
	return args[index+1], index + 1, nil
}

func readPassword(input io.Reader) (string, error) {
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	if err == io.EOF && value == "" {
		return "", fmt.Errorf("no password received")
	}
	return value, nil
}

func printMySQLDDLHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy mysql ddl [connection options]")
	fmt.Fprintln(out)
	printMySQLConnectionHelp(out)
	fmt.Fprintln(out, "  -h, --help                Show this help")
}

func writeMySQLQueryResult(out io.Writer, result mysql.QueryResult, format string) error {
	switch format {
	case "json":
		return json.NewEncoder(out).Encode(result)
	case "table":
		if len(result.Columns) > 0 {
			if _, err := io.WriteString(out, strings.Join(result.Columns, "\t")+"\n"); err != nil {
				return err
			}
		}
		for _, row := range result.Rows {
			values := make([]string, len(row))
			for i, value := range row {
				values[i] = formatMySQLQueryValue(value)
			}
			if _, err := io.WriteString(out, strings.Join(values, "\t")+"\n"); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func formatMySQLQueryValue(value any) string {
	if value == nil {
		return ""
	}
	valueString := fmt.Sprint(value)
	return strings.NewReplacer("\t", "\\t", "\r", "\\r", "\n", "\\n").Replace(valueString)
}

func printMySQLQueryHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy mysql query --sql <statement> [connection options]")
	fmt.Fprintln(out)
	printMySQLConnectionHelp(out)
	fmt.Fprintln(out, "  --sql <statement>         SQL sent to MySQL without CLI filtering")
	fmt.Fprintln(out, "  --format <format>         json or table (default: json)")
	fmt.Fprintln(out, "  -h, --help                Show this help")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Connection defaults load from Home then project configuration; explicit flags override them.")
	fmt.Fprintln(out, "Warning: SQL is executed as provided and may change database contents.")
}

func printMySQLConnectionHelp(out io.Writer) {
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --host <host>             Override mysql.host in configuration")
	fmt.Fprintln(out, "  --port <port>             Override mysql.port (default: 3306)")
	fmt.Fprintln(out, "  --user <user>             Override mysql.user in configuration")
	fmt.Fprintln(out, "  --password <password>     Override mysql.password in configuration")
	fmt.Fprintln(out, "  --password-stdin          Read a password from stdin and override configuration")
	fmt.Fprintln(out, "  --database <database>     Override mysql.database in configuration")
}
