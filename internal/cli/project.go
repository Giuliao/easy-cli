package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Giuliao/easy-cli/internal/project"
	"github.com/spf13/cobra"
)

func (a *app) buildProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects and their associated tasks.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildProjectCreateCmd())
	cmd.AddCommand(a.buildProjectListCmd())
	cmd.AddCommand(a.buildProjectShowCmd())
	cmd.AddCommand(a.buildProjectCloseCmd())
	cmd.AddCommand(a.buildProjectReopenCmd())
	cmd.AddCommand(a.buildProjectDeleteCmd())
	cmd.AddCommand(a.buildProjectUseCmd())
	return cmd
}

func (a *app) buildProjectCreateCmd() *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new project.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProjectCreate(args[0], description)
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "project description")
	return cmd
}

func (a *app) buildProjectListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProjectList(jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func (a *app) buildProjectShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show project details and its tasks.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProjectShow(args[0], jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func (a *app) buildProjectCloseCmd() *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "close <name>",
		Short: "Close a project only when the user explicitly requests it (--confirm).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return a.fail(2, fmt.Errorf("project close requires --confirm and an explicit user request; completing a turn or session does not end the project"))
			}
			return a.runProjectClose(args[0])
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm the user explicitly requested project closure")
	return cmd
}

func (a *app) buildProjectReopenCmd() *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use: "reopen <name>", Short: "Reopen an ended project after explicit user confirmation.", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return a.fail(2, fmt.Errorf("project reopen requires --confirm and an explicit user request"))
			}
			p, err := project.ReopenProject(a.db, args[0])
			if err != nil {
				return a.fail(1, fmt.Errorf("project reopen: %w", err))
			}
			fmt.Fprintf(a.options.Out, "Reopened project %q\n", p.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm the user explicitly requested reopening")
	return cmd
}

func (a *app) buildProjectDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a project.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProjectDelete(args[0])
		},
	}
}

func (a *app) buildProjectUseCmd() *cobra.Command {
	var directory string
	var replace bool
	cmd := &cobra.Command{
		Use:   "use <name>",
		Short: "Bind this worktree to a project; never switch branches.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := project.BindWorkspace(a.db, a.workDirectory(directory), args[0], replace)
			if err != nil {
				return a.fail(1, fmt.Errorf("project use: %w", err))
			}
			fmt.Fprintf(a.options.Out, "Current project set to %q\nWorkspace: %s\nDirectory: %s\nBranch: %s\n", args[0], w.ID, w.Directory, w.Branch)
			return nil
		},
	}
	cmd.Flags().StringVar(&directory, "dir", "", "worktree directory (defaults to current directory)")
	cmd.Flags().BoolVar(&replace, "replace", false, "explicitly replace a binding after user confirmation; clears current task")
	return cmd
}

func (a *app) runProjectCreate(name, description string) error {
	p, err := project.CreateProject(a.db, project.CreateProjectParams{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return a.fail(1, fmt.Errorf("project create: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Created project %q (id: %s)\n", p.Name, p.ID)
	return nil
}

func (a *app) runProjectList(jsonOutput bool) error {
	projects, err := project.ListProjects(a.db)
	if err != nil {
		return a.fail(1, fmt.Errorf("project list: %w", err))
	}
	if jsonOutput {
		return writeJSON(a.options.Out, projects)
	}
	if len(projects) == 0 {
		return nil
	}
	printTable(a.options.Out, []string{"NAME", "STATUS", "CREATED", "DESCRIPTION"},
		func() [][]string {
			rows := make([][]string, 0, len(projects))
			for _, p := range projects {
				rows = append(rows, []string{
					p.Name,
					p.Status,
					p.CreatedAt.Format(time.RFC3339),
					p.Description,
				})
			}
			return rows
		}())
	return nil
}

func (a *app) runProjectShow(name string, jsonOutput bool) error {
	p, err := project.GetProjectByName(a.db, name)
	if err != nil {
		return a.fail(1, fmt.Errorf("project show: %w", err))
	}
	tasks, err := project.ListTasksByProject(a.db, p.ID)
	if err != nil {
		return a.fail(1, fmt.Errorf("project show: %w", err))
	}
	workspaces, err := project.ListWorkspacesByProject(a.db, p.ID)
	if err != nil {
		return a.fail(1, fmt.Errorf("project show: %w", err))
	}
	if jsonOutput {
		payload := struct {
			project.Project
			Tasks      []project.Task      `json:"tasks"`
			Workspaces []project.Workspace `json:"workspaces"`
		}{Project: p, Tasks: tasks, Workspaces: workspaces}
		return writeJSON(a.options.Out, payload)
	}
	fmt.Fprintf(a.options.Out, "Name: %s\n", p.Name)
	fmt.Fprintf(a.options.Out, "ID: %s\n", p.ID)
	fmt.Fprintf(a.options.Out, "Description: %s\n", p.Description)
	fmt.Fprintf(a.options.Out, "Status: %s\n", p.Status)
	fmt.Fprintf(a.options.Out, "Created: %s\n", p.CreatedAt.Format(time.RFC3339))
	if p.EndedAt != nil {
		fmt.Fprintf(a.options.Out, "Ended: %s\n", p.EndedAt.Format(time.RFC3339))
	}
	fmt.Fprintln(a.options.Out)
	fmt.Fprintln(a.options.Out, "Workspaces:")
	if len(workspaces) == 0 {
		fmt.Fprintln(a.options.Out, "  (no bound workspaces; legacy task directories are listed below)")
	} else {
		rows := make([][]string, 0, len(workspaces))
		for _, w := range workspaces {
			rows = append(rows, []string{w.ID, fmt.Sprint(w.Active), w.Repository, w.Branch, w.Directory, w.CurrentTaskID})
		}
		printTable(a.options.Out, []string{"ID", "ACTIVE", "REPOSITORY", "BRANCH", "DIRECTORY", "CURRENT_TASK"}, rows)
	}
	fmt.Fprintln(a.options.Out)
	fmt.Fprintln(a.options.Out, "Tasks:")
	if len(tasks) == 0 {
		fmt.Fprintln(a.options.Out, "  (no tasks)")
		return nil
	}
	printTable(a.options.Out, []string{"ID", "AGENT", "SESSION", "BRANCH", "DIRECTORY", "COMMIT_RANGE"},
		func() [][]string {
			rows := make([][]string, 0, len(tasks))
			for _, t := range tasks {
				rows = append(rows, []string{
					t.ID,
					t.AgentType,
					t.SessionID,
					t.Branch,
					t.Directory,
					t.CommitRange,
				})
			}
			return rows
		}())
	return nil
}

func (a *app) runProjectClose(name string) error {
	p, err := project.CloseProject(a.db, name)
	if err != nil {
		return a.fail(1, fmt.Errorf("project close: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Closed project %q\n", p.Name)
	return nil
}

func (a *app) runProjectDelete(name string) error {
	if err := project.DeleteProject(a.db, name); err != nil {
		return a.fail(1, fmt.Errorf("project delete: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Deleted project %q\n", name)
	return nil
}

func writeJSON(out io.Writer, v any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func printTable(out io.Writer, headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if w := displayWidth(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var line []string
	for i, h := range headers {
		line = append(line, padRight(h, widths[i]))
	}
	fmt.Fprintln(out, strings.Join(line, "   "))
	for _, row := range rows {
		line = line[:0]
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			line = append(line, padRight(cell, widths[i]))
		}
		fmt.Fprintln(out, strings.Join(line, "   "))
	}
}
