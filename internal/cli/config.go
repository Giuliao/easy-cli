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
	switch args[0] {
	case "init":
		return runConfigInit(args[1:], options, out, errOut)
	case "get":
		return runConfigGet(args[1:], options, out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown config command %q\n", args[0])
		printConfigHelp(errOut)
		return 2
	}
}

func runConfigInit(args []string, options Options, out, errOut io.Writer) int {
	force := false
	for _, argument := range args {
		switch argument {
		case "--force":
			if force {
				fmt.Fprintln(errOut, "config init: --force may be specified only once")
				return 2
			}
			force = true
		case "--help", "-h":
			printConfigInitHelp(out)
			return 0
		default:
			fmt.Fprintf(errOut, "config init: unknown option %q\n", argument)
			return 2
		}
	}
	result, err := config.InitHome(config.InitOptions{HomeDir: options.HomeDir, Force: force})
	if err != nil {
		fmt.Fprintf(errOut, "config init: %v\n", err)
		return 1
	}
	if force {
		fmt.Fprintf(out, "Replaced configuration at %s\n", result.Path)
	} else {
		fmt.Fprintf(out, "Initialized configuration at %s\n", result.Path)
	}
	return 0
}

func runConfigGet(args []string, options Options, out, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: easy config get <key>")
		return 2
	}
	if options.ConfigErr != nil {
		fmt.Fprintf(errOut, "config: load configuration: %v\n", options.ConfigErr)
		return 1
	}
	value, err := options.Config.Get(args[0])
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
	fmt.Fprintln(out, "Usage: easy config <command>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Description: Initialize Home configuration and read allowed non-sensitive values.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Configuration files:")
	fmt.Fprintln(out, "  ~/.config/easy-cli/config.json")
	fmt.Fprintln(out, "  <project-root>/.easy-cli/config.json (overrides Home values)")
	fmt.Fprintln(out, "MySQL passwords are never available through config get.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	printAlignedList(out, [][2]string{
		{"init [--force]", "Create the private Home configuration template."},
		{"get <key>", "Print an allowed configuration value."},
	})
}

func printConfigInitHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy config init [--force]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Create ~/.config/easy-cli/config.json with private permissions.")
	fmt.Fprintln(out, "Refuses to replace an existing file unless --force is provided.")
}
