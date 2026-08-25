package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/bytedance/easy-cli/internal/config"
	"github.com/bytedance/easy-cli/internal/mysql"
	"github.com/bytedance/easy-cli/internal/prompt"
	"github.com/bytedance/easy-cli/internal/skill"
	"golang.org/x/term"
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
	printAlignedList(out, [][2]string{
		{"skill list", "List available skills and installation status."},
		{"skill show <name>", "Show skill metadata and installation status."},
		{"skill prompt <name>", "Output the skill's compressed, AI-readable prompt."},
		{"skill install <name>", "Install a skill into the project or user scope."},
		{"skill update <name>", "Update an installed skill from the registered source."},
		{"config init [--force]", "Create the private Home configuration template."},
		{"config get <key>", "Print an allowed non-sensitive configuration value."},
		{"mysql ddl", "Export MySQL base-table CREATE TABLE DDL."},
		{"mysql query", "Execute SQL and output database rows."},
	})
	fprintln(out)
	fmt.Fprintln(out, "Skills:")
	var skillEntries [][2]string
	for _, selected := range registry.List() {
		skillEntries = append(skillEntries, [2]string{selected.Name, selected.Description})
	}
	printAlignedList(out, skillEntries)
}

func printSkillHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy skill <command>")
	fprintln(out)
	fmt.Fprintln(out, "Description: Manage reusable skills and reusable prompts.")
	fprintln(out)
	fmt.Fprintln(out, "Commands:")
	printAlignedList(out, [][2]string{
		{"list", "List available skills and installation status."},
		{"show <name>", "Show skill metadata and installation status."},
		{"prompt <name>", "Output the skill's compressed, AI-readable prompt."},
		{"install <name>", "Install a skill into the project or user scope."},
		{"update <name>", "Update an installed skill from the registered source."},
	})
}

func printAlignedList(out io.Writer, entries [][2]string) {
	nameWidth := 0
	for _, e := range entries {
		if w := displayWidth(e[0]); w > nameWidth {
			nameWidth = w
		}
	}
	nameWidth += 2
	for _, e := range entries {
		fmt.Fprintf(out, "  %s%s\n", padRight(e[0], nameWidth), e[1])
	}
}

func fprintln(out io.Writer) {
	fmt.Fprintln(out)
}

func runList(args []string, registry *skill.Registry, options Options, out, errOut io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(errOut, "usage: easy skill list")
		return 2
	}
	skills := registry.List()
	if len(skills) == 0 {
		return 0
	}

	nameWidth := 0
	for _, s := range skills {
		if w := displayWidth(s.Name); w > nameWidth {
			nameWidth = w
		}
	}
	nameWidth += 2

	const statusWidth = len("not-installed")
	const continuationIndent = 2
	descWidth := terminalWidth() - nameWidth - statusWidth - 1 // -1 for the space before status
	if descWidth < 10 {
		descWidth = 10
	}

	for _, selected := range skills {
		status := "not-installed"
		path, err := skill.InstallPath(selected.Name, skill.InstallOptions{WorkingDir: options.WorkingDir, HomeDir: options.HomeDir})
		if err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				status = "installed"
			}
		}

		lines := wrapText(selected.Description, descWidth-continuationIndent)
		if len(lines) == 0 {
			lines = []string{""}
		}

		fmt.Fprintf(out, "%s%s %s\n", padRight(selected.Name, nameWidth), padRight(lines[0], descWidth), status)
		indent := strings.Repeat(" ", nameWidth+continuationIndent)
		for _, line := range lines[1:] {
			fmt.Fprintf(out, "%s%s\n", indent, line)
		}
	}
	return 0
}

func terminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			return n
		}
	}
	return 80
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeDisplayWidth(r)
	}
	return w
}

func runeDisplayWidth(r rune) int {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK Radicals, Kangxi, Ideographic Description, CJK Symbols
		r >= 0x3041 && r <= 0x33BF, // Hiragana, Katakana, Bopomofo, CJK Strokes, etc.
		r >= 0x3400 && r <= 0x4DBF, // CJK Unified Ideographs Extension A
		r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
		r >= 0xA000 && r <= 0xA4CF, // Yi Syllables, Yi Radicals
		r >= 0xAC00 && r <= 0xD7A3, // Hangul Syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK Compatibility Ideographs
		r >= 0xFE30 && r <= 0xFE4F, // CJK Compatibility Forms
		r >= 0xFF00 && r <= 0xFF60, // Fullwidth Forms
		r >= 0xFFE0 && r <= 0xFFE6: // Fullwidth Signs and Symbols
		return 2
	default:
		return 1
	}
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := ""
	currentWidth := 0
	for _, word := range words {
		wordWidth := displayWidth(word)
		spaceWidth := 0
		if current != "" {
			spaceWidth = 1
		}
		if currentWidth+spaceWidth+wordWidth > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
				currentWidth = 0
			}
			if wordWidth > width {
				for _, r := range word {
					rw := runeDisplayWidth(r)
					if currentWidth+rw > width {
						lines = append(lines, current)
						current = string(r)
						currentWidth = rw
					} else {
						current += string(r)
						currentWidth += rw
					}
				}
			} else {
				current = word
				currentWidth = wordWidth
			}
		} else {
			if current != "" {
				current += " "
			}
			current += word
			currentWidth += spaceWidth + wordWidth
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func padRight(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
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
	fmt.Fprintf(out, "Name: %s\nDescription: %s\nOrigin: %s\nSource: %s\nStatus: %s\n", selected.Name, selected.Description, selected.Origin, selected.SourcePath, status)
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
	selected, err := skill.RenderAggregate(selected, registry)
	if err != nil {
		fmt.Fprintf(errOut, "prepare skill %q: %v\n", name, err)
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
	selected, err := skill.RenderAggregate(selected, registry)
	if err != nil {
		fmt.Fprintf(errOut, "prepare skill %q: %v\n", name, err)
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
	selected, err := skill.RenderAggregate(selected, registry)
	if err != nil {
		fmt.Fprintf(errOut, "prepare skill %q: %v\n", name, err)
		return 1
	}
	compressed, err := prompt.Compress(selected.Source)
	if err != nil {
		fmt.Fprintf(errOut, "compress skill %q: %v\n", selected.Name, err)
		return 1
	}
	if format == "json" {
		payload := struct {
			Name        string       `json:"name"`
			Description string       `json:"description"`
			Origin      skill.Origin `json:"origin"`
			SourcePath  string       `json:"source_path"`
			Raw         string       `json:"raw"`
			Prompt      string       `json:"prompt"`
		}{
			Name:        selected.Name,
			Description: selected.Description,
			Origin:      selected.Origin,
			SourcePath:  selected.SourcePath,
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
