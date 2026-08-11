package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/bytedance/easy-cli/internal/config"
)

func runConfig(args []string, options Options, out, errOut io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		printConfigHelp(out)
		return 0
	}
	if args[0] != "get" {
		fmt.Fprintf(errOut, "unknown config command %q\n", args[0])
		printConfigHelp(errOut)
		return 2
	}
	if len(args) != 2 {
		fmt.Fprintln(errOut, "usage: easy config get <key>")
		return 2
	}
	if options.ConfigErr != nil {
		fmt.Fprintf(errOut, "config: load configuration: %v\n", options.ConfigErr)
		return 1
	}
	value, err := options.Config.Get(args[1])
	if err != nil {
		fmt.Fprintf(errOut, "config get: %v\n", err)
		if errors.Is(err, config.ErrKeyUnavailable) {
			return 2
		}
		return 1
	}
	fmt.Fprintln(out, value)
	return 0
}

func printConfigHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy config get <key>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Description: Read one allowed non-sensitive configuration value.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Configuration files:")
	fmt.Fprintln(out, "  ~/.config/easy-cli/config.json")
	fmt.Fprintln(out, "  <project-root>/.easy-cli/config.json (overrides Home values)")
	fmt.Fprintln(out, "MySQL passwords are never available through config get.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  get <key>                Print an allowed configuration value.")
}
