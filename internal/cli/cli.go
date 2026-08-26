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
	"github.com/spf13/cobra"
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

type app struct {
	registry *skill.Registry
	options  Options
	exitCode int
}

type onceBool struct {
	value *bool
	set   bool
}

func (o *onceBool) String() string {
	if o.value == nil {
		return "false"
	}
	return strconv.FormatBool(*o.value)
}

func (o *onceBool) Set(v string) error {
	if o.set {
		return fmt.Errorf("--force may be specified only once")
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	*o.value = b
	o.set = true
	return nil
}

func (o *onceBool) Type() string {
	return "bool"
}

func Run(args []string, registry *skill.Registry, options Options) int {
	if options.Out == nil {
		options.Out = io.Discard
	}
	if options.ErrOut == nil {
		options.ErrOut = io.Discard
	}
	a := &app{registry: registry, options: options}
	rootCmd := a.buildRootCmd()
	rootCmd.SetArgs(args)
	rootCmd.SetOut(options.Out)
	rootCmd.SetErr(options.ErrOut)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(options.ErrOut, err)
		if a.exitCode == 0 {
			a.exitCode = 1
		}
	}
	return a.exitCode
}

func (a *app) buildRootCmd() *cobra.Command {
	out := a.options.Out

	rootCmd := &cobra.Command{
		Use:   "easy",
		Short: "Manage reusable AI skills and MySQL database access.",
		RunE: func(cmd *cobra.Command, args []string) error {
			a.printRootHelp(out)
			return nil
		},
	}
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		a.printRootHelp(out)
	})
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		a.exitCode = 2
		return err
	})

	rootCmd.AddCommand(a.buildMySQLCmd())
	rootCmd.AddCommand(a.buildConfigCmd())
	rootCmd.AddCommand(a.buildSkillCmd())

	// Register dynamic skills as top-level commands.
	for _, s := range a.registry.List() {
		skill := s
		cmd := &cobra.Command{
			Use:   skill.Name,
			Short: skill.Description,
			RunE: func(cmd *cobra.Command, args []string) error {
				return a.runSkillPrompt(cmd, skill.Name)
			},
		}
		cmd.Flags().Bool("raw", false, "output raw skill source")
		cmd.Flags().String("format", "text", "output format: text or json")
		rootCmd.AddCommand(cmd)
	}

	return rootCmd
}

func (a *app) buildMySQLCmd() *cobra.Command {
	mysqlCmd := &cobra.Command{
		Use:   "mysql",
		Short: "MySQL database access commands.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	mysqlCmd.AddCommand(a.buildMySQLDDLCmd())
	mysqlCmd.AddCommand(a.buildMySQLQueryCmd())
	return mysqlCmd
}

func (a *app) buildMySQLDDLCmd() *cobra.Command {
	var overrides mysqlConnectionOverrides
	cmd := &cobra.Command{
		Use:   "ddl",
		Short: "Export MySQL base-table CREATE TABLE DDL.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runMySQLDDL(cmd, &overrides)
		},
	}
	addMySQLConnectionFlags(cmd, &overrides)
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printMySQLDDLHelp(a.options.Out)
	})
	return cmd
}

func (a *app) buildMySQLQueryCmd() *cobra.Command {
	var overrides mysqlConnectionOverrides
	var statement string
	var format string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Execute SQL and output database rows.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runMySQLQuery(cmd, &overrides, statement, format)
		},
	}
	addMySQLConnectionFlags(cmd, &overrides)
	cmd.Flags().Var(&namedStringFlag{value: &statement, typeName: "<statement>"}, "sql", "SQL sent to MySQL without CLI filtering")
	cmd.Flags().StringVar(&format, "format", "json", "json or table")
	_ = cmd.MarkFlagRequired("sql")
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printMySQLQueryHelp(a.options.Out)
	})
	return cmd
}

func (a *app) buildConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Initialize Home configuration and read allowed non-sensitive values.",
		RunE: func(cmd *cobra.Command, args []string) error {
			printConfigHelp(a.options.Out)
			return nil
		},
	}
	configCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printConfigHelp(a.options.Out)
	})
	configCmd.AddCommand(a.buildConfigInitCmd())
	configCmd.AddCommand(a.buildConfigGetCmd())
	return configCmd
}

func (a *app) buildConfigInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init [--force]",
		Short: "Create the private Home configuration template.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runConfigInit(force)
		},
	}
	forceFlag := cmd.Flags().VarPF(&onceBool{value: &force}, "force", "", "overwrite an existing configuration file")
	forceFlag.NoOptDefVal = "true"
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printConfigInitHelp(a.options.Out)
	})
	return cmd
}

func (a *app) buildConfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print an allowed non-sensitive configuration value.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runConfigGet(args[0])
		},
	}
	return cmd
}

func (a *app) buildSkillCmd() *cobra.Command {
	skillCmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage reusable skills and reusable prompts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			printSkillHelp(a.options.Out)
			return nil
		},
	}
	skillCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printSkillHelp(a.options.Out)
	})
	skillCmd.AddCommand(a.buildSkillListCmd())
	skillCmd.AddCommand(a.buildSkillShowCmd())
	skillCmd.AddCommand(a.buildSkillInstallCmd())
	skillCmd.AddCommand(a.buildSkillUpdateCmd())
	skillCmd.AddCommand(a.buildSkillPromptCmd())
	return skillCmd
}

func (a *app) buildSkillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available skills and installation status.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSkillList()
		},
	}
}

func (a *app) buildSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show skill metadata and installation status.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSkillShow(args[0])
		},
	}
}

func (a *app) buildSkillInstallCmd() *cobra.Command {
	var global, force bool
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a skill into the project or user scope.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSkillInstall(args[0], global, force)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "install to the user scope")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing installation")
	return cmd
}

func (a *app) buildSkillUpdateCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an installed skill from the registered source.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSkillUpdate(args[0], global)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "update the user-scope installation")
	return cmd
}

func (a *app) buildSkillPromptCmd() *cobra.Command {
	var raw bool
	var format string
	cmd := &cobra.Command{
		Use:   "prompt <name>",
		Short: "Output the skill's compressed, AI-readable prompt.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSkillPromptByName(args[0], raw, format)
		},
	}
	cmd.Flags().BoolVar(&raw, "raw", false, "output raw skill source")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

func (a *app) fail(code int, err error) error {
	a.exitCode = code
	return err
}

func (a *app) runMySQLDDL(cmd *cobra.Command, overrides *mysqlConnectionOverrides) error {
	if a.options.ConfigErr != nil {
		return a.fail(1, fmt.Errorf("mysql ddl: load configuration: %w", a.options.ConfigErr))
	}
	overrides.applyFlagChanges(cmd)
	connection, err := resolveMySQLConnection(a.options.Config.MySQL, *overrides)
	if err != nil {
		return a.fail(2, fmt.Errorf("mysql ddl: %w", err))
	}
	if overrides.PasswordStdin {
		input := a.options.In
		if input == nil {
			input = os.Stdin
		}
		connection.Password, err = readPassword(input)
		if err != nil {
			return a.fail(1, fmt.Errorf("mysql ddl: read password: %w", err))
		}
	}
	exporter := a.options.MySQLExport
	if exporter == nil {
		exporter = mysql.ExportDDL
	}
	ctx, cancel := context.WithTimeout(context.Background(), mySQLExportTimeout)
	defer cancel()
	if err := exporter(ctx, connection, a.options.Out); err != nil {
		return a.fail(1, fmt.Errorf("mysql ddl: export failed: %w", err))
	}
	return nil
}

func (a *app) runMySQLQuery(cmd *cobra.Command, overrides *mysqlConnectionOverrides, statement, format string) error {
	if a.options.ConfigErr != nil {
		return a.fail(1, fmt.Errorf("mysql query: load configuration: %w", a.options.ConfigErr))
	}
	if statement == "" || statement == "--help" || statement == "-h" || statement == "--format" {
		return a.fail(2, fmt.Errorf("mysql query: --sql requires a non-empty value"))
	}
	if format != "json" && format != "table" {
		return a.fail(2, fmt.Errorf("mysql query: --format must be json or table"))
	}
	overrides.applyFlagChanges(cmd)
	connection, err := resolveMySQLConnection(a.options.Config.MySQL, *overrides)
	if err != nil {
		return a.fail(2, fmt.Errorf("mysql query: %w", err))
	}
	if overrides.PasswordStdin {
		input := a.options.In
		if input == nil {
			input = os.Stdin
		}
		connection.Password, err = readPassword(input)
		if err != nil {
			return a.fail(1, fmt.Errorf("mysql query: read password: %w", err))
		}
	}
	query := a.options.MySQLQuery
	if query == nil {
		query = mysql.Query
	}
	ctx, cancel := context.WithTimeout(context.Background(), mySQLExportTimeout)
	defer cancel()
	result, err := query(ctx, connection, statement)
	if err != nil {
		return a.fail(1, fmt.Errorf("mysql query: execution failed: %w", err))
	}
	if err := writeMySQLQueryResult(a.options.Out, result, format); err != nil {
		return a.fail(1, fmt.Errorf("mysql query: format result: %w", err))
	}
	return nil
}

func (a *app) runSkillList() error {
	skills := a.registry.List()
	if len(skills) == 0 {
		return nil
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
	descWidth := terminalWidth() - nameWidth - statusWidth - 1
	if descWidth < 10 {
		descWidth = 10
	}

	for _, selected := range skills {
		status := "not-installed"
		path, err := skill.InstallPath(selected.Name, skill.InstallOptions{WorkingDir: a.options.WorkingDir, HomeDir: a.options.HomeDir})
		if err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				status = "installed"
			}
		}

		lines := wrapText(selected.Description, descWidth-continuationIndent)
		if len(lines) == 0 {
			lines = []string{""}
		}

		fmt.Fprintf(a.options.Out, "%s%s %s\n", padRight(selected.Name, nameWidth), padRight(lines[0], descWidth), status)
		indent := strings.Repeat(" ", nameWidth+continuationIndent)
		for _, line := range lines[1:] {
			fmt.Fprintf(a.options.Out, "%s%s\n", indent, line)
		}
	}
	return nil
}

func (a *app) runSkillShow(name string) error {
	selected, ok := a.registry.Get(name)
	if !ok {
		return a.fail(1, fmt.Errorf("unknown skill %q", name))
	}
	status := "not-installed"
	path, err := skill.InstallPath(selected.Name, skill.InstallOptions{WorkingDir: a.options.WorkingDir, HomeDir: a.options.HomeDir})
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			status = "installed"
		}
	}
	fmt.Fprintf(a.options.Out, "Name: %s\nDescription: %s\nOrigin: %s\nSource: %s\nStatus: %s\n",
		selected.Name, selected.Description, selected.Origin, selected.SourcePath, status)
	return nil
}

func (a *app) runSkillInstall(name string, global, force bool) error {
	selected, ok := a.registry.Get(name)
	if !ok {
		return a.fail(1, fmt.Errorf("unknown skill %q", name))
	}
	selected, err := skill.RenderAggregate(selected, a.registry)
	if err != nil {
		return a.fail(1, fmt.Errorf("prepare skill %q: %w", name, err))
	}
	result, err := skill.Install(selected, skill.InstallOptions{
		Global:     global,
		Force:      force,
		WorkingDir: a.options.WorkingDir,
		HomeDir:    a.options.HomeDir,
	})
	if err != nil {
		return a.fail(1, fmt.Errorf("install skill %q: %w", name, err))
	}
	if result.Changed {
		fmt.Fprintf(a.options.Out, "Installed %s to %s\n", name, result.Path)
	} else {
		fmt.Fprintf(a.options.Out, "Already installed %s at %s\n", name, result.Path)
	}
	return nil
}

func (a *app) runSkillUpdate(name string, global bool) error {
	selected, ok := a.registry.Get(name)
	if !ok {
		return a.fail(1, fmt.Errorf("unknown skill %q", name))
	}
	selected, err := skill.RenderAggregate(selected, a.registry)
	if err != nil {
		return a.fail(1, fmt.Errorf("prepare skill %q: %w", name, err))
	}
	result, err := skill.Update(selected, skill.InstallOptions{
		Global:     global,
		WorkingDir: a.options.WorkingDir,
		HomeDir:    a.options.HomeDir,
	})
	if err != nil {
		return a.fail(1, fmt.Errorf("update skill %q: %w", name, err))
	}
	if result.Changed {
		fmt.Fprintf(a.options.Out, "Updated %s to %s\n", name, result.Path)
	} else {
		fmt.Fprintf(a.options.Out, "Already up to date %s at %s\n", name, result.Path)
	}
	return nil
}

func (a *app) runSkillPrompt(cmd *cobra.Command, name string) error {
	raw, _ := cmd.Flags().GetBool("raw")
	format, _ := cmd.Flags().GetString("format")
	return a.runSkillPromptByName(name, raw, format)
}

func (a *app) runSkillPromptByName(name string, raw bool, format string) error {
	selected, ok := a.registry.Get(name)
	if !ok {
		return a.fail(1, fmt.Errorf("unknown skill %q", name))
	}
	selected, err := skill.RenderAggregate(selected, a.registry)
	if err != nil {
		return a.fail(1, fmt.Errorf("prepare skill %q: %w", name, err))
	}
	compressed, err := prompt.Compress(selected.Source)
	if err != nil {
		return a.fail(1, fmt.Errorf("compress skill %q: %w", selected.Name, err))
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
		if err := json.NewEncoder(a.options.Out).Encode(payload); err != nil {
			return a.fail(1, fmt.Errorf("write JSON output: %w", err))
		}
		return nil
	}
	if format != "text" {
		return a.fail(2, fmt.Errorf("unsupported format %q", format))
	}
	if raw {
		_, _ = io.WriteString(a.options.Out, selected.Source)
		return nil
	}
	_, _ = io.WriteString(a.options.Out, compressed)
	return nil
}

func (a *app) printRootHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy <command>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Description: Manage reusable AI skills and MySQL database access.")
	fmt.Fprintln(out)
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
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skills:")
	var skillEntries [][2]string
	for _, selected := range a.registry.List() {
		skillEntries = append(skillEntries, [2]string{selected.Name, selected.Description})
	}
	printAlignedList(out, skillEntries)
}

func printSkillHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: easy skill <command>")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Description: Manage reusable skills and reusable prompts.")
	fmt.Fprintln(out)
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
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0x303E,
		r >= 0x3041 && r <= 0x33BF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA000 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6:
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
