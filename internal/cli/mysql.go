package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/easy-cli/internal/mysql"
)

const defaultMySQLPort = 3306
const mySQLExportTimeout = 30 * time.Second

func runMySQLDDL(args []string, options Options, out, errOut io.Writer) int {
	connection, passwordStdin, help, err := parseMySQLDDLArgs(args)
	if err != nil {
		fmt.Fprintf(errOut, "mysql ddl: %v\n", err)
		return 2
	}
	if help {
		printMySQLDDLHelp(out)
		return 0
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

func parseMySQLDDLArgs(args []string) (mysql.ConnectionOptions, bool, bool, error) {
	options := mysql.ConnectionOptions{Port: defaultMySQLPort}
	passwordSet := false
	passwordStdin := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			value, next, err := requiredFlagValue(args, i, "--host")
			if err != nil {
				return mysql.ConnectionOptions{}, false, false, err
			}
			options.Host, i = value, next
		case "--port":
			value, next, err := requiredFlagValue(args, i, "--port")
			if err != nil {
				return mysql.ConnectionOptions{}, false, false, err
			}
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return mysql.ConnectionOptions{}, false, false, fmt.Errorf("--port must be between 1 and 65535")
			}
			options.Port, i = port, next
		case "--user":
			value, next, err := requiredFlagValue(args, i, "--user")
			if err != nil {
				return mysql.ConnectionOptions{}, false, false, err
			}
			options.User, i = value, next
		case "--password":
			if passwordStdin {
				return mysql.ConnectionOptions{}, false, false, fmt.Errorf("--password and --password-stdin are mutually exclusive")
			}
			value, next, err := requiredFlagValue(args, i, "--password")
			if err != nil {
				return mysql.ConnectionOptions{}, false, false, err
			}
			options.Password, passwordSet, i = value, true, next
		case "--password-stdin":
			if passwordSet {
				return mysql.ConnectionOptions{}, false, false, fmt.Errorf("--password and --password-stdin are mutually exclusive")
			}
			passwordSet = true
			passwordStdin = true
		case "--database":
			value, next, err := requiredFlagValue(args, i, "--database")
			if err != nil {
				return mysql.ConnectionOptions{}, false, false, err
			}
			options.Database, i = value, next
		case "--help", "-h":
			return mysql.ConnectionOptions{}, false, true, nil
		default:
			return mysql.ConnectionOptions{}, false, false, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if options.Host == "" {
		return mysql.ConnectionOptions{}, false, false, fmt.Errorf("--host is required")
	}
	if options.User == "" {
		return mysql.ConnectionOptions{}, false, false, fmt.Errorf("--user is required")
	}
	if options.Database == "" {
		return mysql.ConnectionOptions{}, false, false, fmt.Errorf("--database is required")
	}
	if !passwordSet {
		return mysql.ConnectionOptions{}, false, false, fmt.Errorf("one of --password or --password-stdin is required")
	}
	return options, passwordStdin, false, nil
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
	fmt.Fprintln(out, "Usage: easy mysql ddl --host <host> --user <user> --database <database> [options]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --host <host>             MySQL host")
	fmt.Fprintln(out, "  --port <port>             MySQL port (default: 3306)")
	fmt.Fprintln(out, "  --user <user>             MySQL user")
	fmt.Fprintln(out, "  --password <password>     MySQL password")
	fmt.Fprintln(out, "  --password-stdin          Read password from stdin")
	fmt.Fprintln(out, "  --database <database>     Database name")
	fmt.Fprintln(out, "  -h, --help                Show this help")
}
