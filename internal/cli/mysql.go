package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bytedance/easy-cli/internal/config"
	"github.com/bytedance/easy-cli/internal/mysql"
	"github.com/spf13/cobra"
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

type namedStringFlag struct {
	value    *string
	typeName string
}

func (n *namedStringFlag) String() string {
	if n.value == nil {
		return ""
	}
	return *n.value
}

func (n *namedStringFlag) Set(v string) error {
	*n.value = v
	return nil
}

func (n *namedStringFlag) Type() string {
	return n.typeName
}

func addMySQLConnectionFlags(cmd *cobra.Command, overrides *mysqlConnectionOverrides) {
	cmd.Flags().StringVar(&overrides.Host, "host", "", "Override mysql.host in configuration")
	cmd.Flags().IntVar(&overrides.Port, "port", 0, "Override mysql.port (default: 3306)")
	cmd.Flags().StringVar(&overrides.User, "user", "", "Override mysql.user in configuration")
	cmd.Flags().StringVar(&overrides.Password, "password", "", "Override mysql.password in configuration")
	cmd.Flags().BoolVar(&overrides.PasswordStdin, "password-stdin", false, "Read a password from stdin and override configuration")
	cmd.Flags().StringVar(&overrides.Database, "database", "", "Override mysql.database in configuration")
}

func (o *mysqlConnectionOverrides) applyFlagChanges(cmd *cobra.Command) {
	o.HostSet = cmd.Flags().Changed("host")
	o.PortSet = cmd.Flags().Changed("port")
	o.UserSet = cmd.Flags().Changed("user")
	o.PasswordSet = cmd.Flags().Changed("password")
	o.DatabaseSet = cmd.Flags().Changed("database")
}

func resolveMySQLConnection(configured config.MySQL, overrides mysqlConnectionOverrides) (mysql.ConnectionOptions, error) {
	if overrides.PasswordSet && overrides.PasswordStdin {
		return mysql.ConnectionOptions{}, fmt.Errorf("--password and --password-stdin are mutually exclusive")
	}
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

func printMySQLDDLHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy mysql ddl [connection options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	printAlignedList(out, append(mysqlConnectionOptions(),
		[2]string{"-h, --help", "Show this help"},
	))
}

func printMySQLQueryHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy mysql query --sql <statement> [connection options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	printAlignedList(out, append(mysqlConnectionOptions(),
		[2]string{"--sql <statement>", "SQL sent to MySQL without CLI filtering"},
		[2]string{"--format <format>", "json or table (default: json)"},
		[2]string{"-h, --help", "Show this help"},
	))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Connection defaults load from Home then project configuration; explicit flags override them.")
	fmt.Fprintln(out, "Warning: SQL is executed as provided and may change database contents.")
}

func mysqlConnectionOptions() [][2]string {
	return [][2]string{
		{"--host <host>", "Override mysql.host in configuration"},
		{"--port <port>", "Override mysql.port (default: 3306)"},
		{"--user <user>", "Override mysql.user in configuration"},
		{"--password <password>", "Override mysql.password in configuration"},
		{"--password-stdin", "Read a password from stdin and override configuration"},
		{"--database <database>", "Override mysql.database in configuration"},
	}
}
