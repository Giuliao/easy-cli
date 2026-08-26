package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/bytedance/easy-cli/internal/config"
)

func (a *app) runConfigInit(force bool) error {
	result, err := config.InitHome(config.InitOptions{HomeDir: a.options.HomeDir, Force: force})
	if err != nil {
		return a.fail(1, fmt.Errorf("config init: %w", err))
	}
	if force {
		fmt.Fprintf(a.options.Out, "Replaced configuration at %s\n", result.Path)
	} else {
		fmt.Fprintf(a.options.Out, "Initialized configuration at %s\n", result.Path)
	}
	return nil
}

func (a *app) runConfigGet(key string) error {
	if a.options.ConfigErr != nil {
		return a.fail(1, fmt.Errorf("config: load configuration: %w", a.options.ConfigErr))
	}
	value, err := a.options.Config.Get(key)
	if err != nil {
		if errors.Is(err, config.ErrKeyUnavailable) {
			return a.fail(2, fmt.Errorf("config get: %w", err))
		}
		return a.fail(1, fmt.Errorf("config get: %w", err))
	}
	fmt.Fprintln(a.options.Out, value)
	return nil
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
