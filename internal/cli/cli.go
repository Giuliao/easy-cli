package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/bytedance/easy-cli/internal/config"
	"github.com/bytedance/easy-cli/internal/mysql"
	"github.com/bytedance/easy-cli/internal/prompt"
	"github.com/bytedance/easy-cli/internal/skill"
)

type Options struct {
	WorkingDir  string
	HomeDir     string
	Config      config.Config
	ConfigErr   error
	In          io.Reader
	Out         io.Writer
	ErrOut      io.Writer
	MySQLExport func(context.Context, mysql.ConnectionOptions, io.Writer) error
	MySQLQuery  func(context.Context, mysql.ConnectionOptions, string) (mysql.QueryResult, error)
}

func Run(args []string, registry *skill.Registry, options Options) int {
	out := options.Out
	if out == nil {
		out = io.Discard
	}
	errOut := options.ErrOut
	if errOut == nil {
		errOut = io.Discard
	}
	if len(args) >= 2 && args[0] == "mysql" && args[1] == "ddl" {
		return runMySQLDDL(args[2:], options, out, errOut)
	}
	if len(args) >= 2 && args[0] == "mysql" && args[1] == "query" {
		return runMySQLQuery(args[2:], options, out, errOut)
	}
	if len(args) >= 1 && args[0] == "config" {
		return runConfig(args[1:], options, out, errOut)
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(out, registry)
		return 0
	}
	if len(args) == 1 && args[0] == "skill" {
		printSkillHelp(out)
		return 0
	}
	if len(args) >= 2 && args[0] == "skill" {
		switch args[1] {
		case "help", "--help", "-h":
			printSkillHelp(out)
			return 0
		case "list":
			return runList(args[2:], registry, options, out, errOut)
		case "show":
			return runShow(args[2:], registry, options, out, errOut)
		case "install":
			return runInstall(args[2:], registry, options, out, errOut)
		case "update":
			return runUpdate(args[2:], registry, options, out, errOut)
		case "prompt":
			return runPrompt(args[2:], registry, out, errOut)
		}
	}
	if len(args) > 0 {
		if _, ok := registry.Get(args[0]); ok {
			return runPrompt(args, registry, out, errOut)
		}
	}
	fmt.Fprintf(errOut, "unknown command %q\n", args[0])
	printHelp(errOut, registry)
	return 2
}

func printHelp(out io.Writer, registry *skill.Registry) {
	fmt.Fprintln(out, "Usage: easy <command>")
	fprintln(out)
	fmt.Fprintln(out, "Description: Manage reusable AI skills and MySQL database access.")
	fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  skill list                List available skills and installation status.")
	fmt.Fprintln(out, "  skill show <name>         Show skill metadata and installation status.")
	fmt.Fprintln(out, "  skill prompt <name>       Output the skill's compressed, AI-readable prompt.")
	fmt.Fprintln(out, "  skill install <name>      Install a skill into the project or user scope.")
	fmt.Fprintln(out, "  skill update <name>       Update an installed skill from the embedded source.")
	fmt.Fprintln(out, "  config init [--force]     Create the private Home configuration template.")
	fmt.Fprintln(out, "  config get <key>          Print an allowed non-sensitive configuration value.")
	fmt.Fprintln(out, "  mysql ddl                 Export MySQL base-table CREATE TABLE DDL.")
	fmt.Fprintln(out, "  mysql query               Execute SQL and output database rows.")
	fprintln(out)
	fmt.Fprintln(out, "Skills:")
	for _, selected := range registry.List() {
		fmt.Fprintf(out, "  %-25s %s\n", selected.Name, selected.Description)
	}
}

func printSkillHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy skill <command>")
	fprintln(out)
	fmt.Fprintln(out, "Description: Manage reusable skills and reusable prompts.")
	fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  list                     List available skills and installation status.")
	fmt.Fprintln(out, "  show <name>              Show skill metadata and installation status.")
	fmt.Fprintln(out, "  prompt <name>            Output the skill's compressed, AI-readable prompt.")
	fmt.Fprintln(out, "  install <name>           Install a skill into the project or user scope.")
	fmt.Fprintln(out, "  update <name>            Update an installed skill from the embedded source.")
}

func fprintln(out io.Writer) {
	fmt.Fprintln(out)
}

func runList(args []string, registry *skill.Registry, options Options, out, errOut io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(errOut, "usage: easy skill list")
		return 2
	}
	for _, selected := range registry.List() {
		status := "not-installed"
		path, err := skill.InstallPath(selected.Name, skill.InstallOptions{WorkingDir: options.WorkingDir, HomeDir: options.HomeDir})
		if err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				status = "installed"
			}
		}
		fmt.Fprintf(out, "%s\t%s\t%s\n", selected.Name, selected.Description, status)
	}
	return 0
}

func runShow(args []string, registry *skill.Registry, options Options, out, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "usage: easy skill show <name>")
		return 2
	}
	selected, ok := registry.Get(args[0])
	if !ok {
		fmt.Fprintf(errOut, "unknown skill %q\n", args[0])
		return 1
	}
	status := "not-installed"
	path, err := skill.InstallPath(selected.Name, skill.InstallOptions{WorkingDir: options.WorkingDir, HomeDir: options.HomeDir})
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			status = "installed"
		}
	}
	fmt.Fprintf(out, "Name: %s\nDescription: %s\nStatus: %s\n", selected.Name, selected.Description, status)
	return 0
}

func runInstall(args []string, registry *skill.Registry, options Options, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: easy skill install <name> [--global] [--force]")
		return 2
	}
	name := args[0]
	global := false
	force := false
	for _, argument := range args[1:] {
		switch argument {
		case "--global":
			global = true
		case "--force":
			force = true
		case "--help", "-h":
			fmt.Fprintln(out, "usage: easy skill install <name> [--global] [--force]")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown option %q\n", argument)
			return 2
		}
	}
	selected, ok := registry.Get(name)
	if !ok {
		fmt.Fprintf(errOut, "unknown skill %q\n", name)
		return 1
	}
	result, err := skill.Install(selected, skill.InstallOptions{
		Global:     global,
		Force:      force,
		WorkingDir: options.WorkingDir,
		HomeDir:    options.HomeDir,
	})
	if err != nil {
		fmt.Fprintf(errOut, "install skill %q: %v\n", name, err)
		return 1
	}
	if result.Changed {
		fmt.Fprintf(out, "Installed %s to %s\n", name, result.Path)
	} else {
		fmt.Fprintf(out, "Already installed %s at %s\n", name, result.Path)
	}
	return 0
}

func runUpdate(args []string, registry *skill.Registry, options Options, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: easy skill update <name> [--global]")
		return 2
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "usage: easy skill update <name> [--global]")
		return 0
	}
	name := args[0]
	global := false
	for _, argument := range args[1:] {
		switch argument {
		case "--global":
			global = true
		case "--help", "-h":
			fmt.Fprintln(out, "usage: easy skill update <name> [--global]")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown option %q\n", argument)
			return 2
		}
	}
	selected, ok := registry.Get(name)
	if !ok {
		fmt.Fprintf(errOut, "unknown skill %q\n", name)
		return 1
	}
	result, err := skill.Update(selected, skill.InstallOptions{
		Global:     global,
		WorkingDir: options.WorkingDir,
		HomeDir:    options.HomeDir,
	})
	if err != nil {
		fmt.Fprintf(errOut, "update skill %q: %v\n", name, err)
		return 1
	}
	if result.Changed {
		fmt.Fprintf(out, "Updated %s to %s\n", name, result.Path)
	} else {
		fmt.Fprintf(out, "Already up to date %s at %s\n", name, result.Path)
	}
	return 0
}

func runPrompt(args []string, registry *skill.Registry, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: easy skill prompt <name> [--raw|--format json]")
		return 2
	}
	name := args[0]
	raw := false
	format := "text"
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--raw":
			raw = true
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(errOut, "--format requires a value")
				return 2
			}
			i++
			format = args[i]
		case "--help", "-h":
			fmt.Fprintln(out, "usage: easy skill prompt <name> [--raw|--format json]")
			return 0
		default:
			fmt.Fprintf(errOut, "unknown option %q\n", args[i])
			return 2
		}
	}
	selected, ok := registry.Get(name)
	if !ok {
		fmt.Fprintf(errOut, "unknown skill %q\n", name)
		return 1
	}
	compressed, err := prompt.Compress(selected.Source)
	if err != nil {
		fmt.Fprintf(errOut, "compress skill %q: %v\n", selected.Name, err)
		return 1
	}
	if format == "json" {
		payload := struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Raw         string `json:"raw"`
			Prompt      string `json:"prompt"`
		}{
			Name:        selected.Name,
			Description: selected.Description,
			Raw:         selected.Source,
			Prompt:      compressed,
		}
		if err := json.NewEncoder(out).Encode(payload); err != nil {
			fmt.Fprintf(errOut, "write JSON output: %v\n", err)
			return 1
		}
		return 0
	}
	if format != "text" {
		fmt.Fprintf(errOut, "unsupported format %q\n", format)
		return 2
	}
	if raw {
		_, _ = io.WriteString(out, selected.Source)
		return 0
	}
	_, _ = io.WriteString(out, compressed)
	return 0
}
