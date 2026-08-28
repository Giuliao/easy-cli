package cli

import (
	"fmt"
	"time"

	"github.com/Giuliao/easy-cli/internal/project"
	"github.com/spf13/cobra"
)

func (a *app) buildTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage agent development tasks.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildTaskCreateCmd())
	cmd.AddCommand(a.buildTaskListCmd())
	cmd.AddCommand(a.buildTaskShowCmd())
	cmd.AddCommand(a.buildTaskUpdateCmd())
	cmd.AddCommand(a.buildTaskDeleteCmd())
	cmd.AddCommand(a.buildTaskAttachCmd())
	cmd.AddCommand(a.buildTaskDetachCmd())
	return cmd
}

func (a *app) buildTaskCreateCmd() *cobra.Command {
	var agentType, sessionID, branch, directory, commitRange, projectName string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new task.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTaskCreate(agentType, sessionID, branch, directory, commitRange, projectName)
		},
	}
	cmd.Flags().StringVar(&agentType, "agent", "", "agent type (e.g. codex, claude)")
	cmd.Flags().StringVar(&sessionID, "session", "", "agent session ID")
	cmd.Flags().StringVar(&branch, "branch", "", "development branch")
	cmd.Flags().StringVar(&directory, "dir", "", "development directory")
	cmd.Flags().StringVar(&commitRange, "commit-range", "", "git commit range (e.g. abc..def)")
	cmd.Flags().StringVar(&projectName, "project", "", "project name to attach to")
	_ = cmd.MarkFlagRequired("agent")
	_ = cmd.MarkFlagRequired("session")
	_ = cmd.MarkFlagRequired("branch")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

func (a *app) buildTaskListCmd() *cobra.Command {
	var projectName string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTaskList(projectName, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "filter by project name")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func (a *app) buildTaskShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTaskShow(args[0], jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
	return cmd
}

func (a *app) buildTaskUpdateCmd() *cobra.Command {
	var agentType, sessionID, branch, directory, commitRange string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update task fields.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := project.UpdateTaskParams{}
			if cmd.Flags().Changed("agent") {
				params.AgentType = &agentType
			}
			if cmd.Flags().Changed("session") {
				params.SessionID = &sessionID
			}
			if cmd.Flags().Changed("branch") {
				params.Branch = &branch
			}
			if cmd.Flags().Changed("dir") {
				params.Directory = &directory
			}
			if cmd.Flags().Changed("commit-range") {
				params.CommitRange = &commitRange
			}
			return a.runTaskUpdate(args[0], params)
		},
	}
	cmd.Flags().StringVar(&agentType, "agent", "", "agent type")
	cmd.Flags().StringVar(&sessionID, "session", "", "agent session ID")
	cmd.Flags().StringVar(&branch, "branch", "", "development branch")
	cmd.Flags().StringVar(&directory, "dir", "", "development directory")
	cmd.Flags().StringVar(&commitRange, "commit-range", "", "git commit range")
	return cmd
}

func (a *app) buildTaskDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a task.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTaskDelete(args[0])
		},
	}
}

func (a *app) buildTaskAttachCmd() *cobra.Command {
	var projectName string
	cmd := &cobra.Command{
		Use:   "attach <task-id>",
		Short: "Attach a task to a project.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTaskAttach(args[0], projectName)
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "project name")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func (a *app) buildTaskDetachCmd() *cobra.Command {
	var projectName string
	cmd := &cobra.Command{
		Use:   "detach <task-id>",
		Short: "Detach a task from a project.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTaskDetach(args[0], projectName)
		},
	}
	cmd.Flags().StringVar(&projectName, "project", "", "project name")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func (a *app) runTaskCreate(agentType, sessionID, branch, directory, commitRange, projectName string) error {
	if projectName == "" {
		projectName = project.CurrentProject(a.options.WorkingDir)
	}
	t, err := project.CreateTask(a.db, project.CreateTaskParams{
		AgentType:   agentType,
		SessionID:   sessionID,
		Branch:      branch,
		Directory:   directory,
		CommitRange: commitRange,
	})
	if err != nil {
		return a.fail(1, fmt.Errorf("task create: %w", err))
	}
	if projectName != "" {
		p, err := project.GetProjectByName(a.db, projectName)
		if err != nil {
			return a.fail(1, fmt.Errorf("task create: %w", err))
		}
		if err := project.AttachTaskToProject(a.db, t.ID, p.ID); err != nil {
			return a.fail(1, fmt.Errorf("task create: attach to project: %w", err))
		}
		fmt.Fprintf(a.options.Out, "Created task %s and attached to project %q\n", t.ID, projectName)
		return nil
	}
	fmt.Fprintf(a.options.Out, "Created task %s\n", t.ID)
	return nil
}

func (a *app) runTaskList(projectName string, jsonOutput bool) error {
	var tasks []project.Task
	var err error
	if projectName != "" {
		p, err := project.GetProjectByName(a.db, projectName)
		if err != nil {
			return a.fail(1, fmt.Errorf("task list: %w", err))
		}
		tasks, err = project.ListTasksByProject(a.db, p.ID)
	} else {
		tasks, err = project.ListTasks(a.db)
	}
	if err != nil {
		return a.fail(1, fmt.Errorf("task list: %w", err))
	}
	if jsonOutput {
		return writeJSON(a.options.Out, tasks)
	}
	if len(tasks) == 0 {
		return nil
	}
	printTable(a.options.Out, []string{"ID", "AGENT", "SESSION", "BRANCH", "DIRECTORY", "COMMIT_RANGE", "UPDATED"},
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
					t.UpdatedAt.Format(time.RFC3339),
				})
			}
			return rows
		}())
	return nil
}

func (a *app) runTaskShow(id string, jsonOutput bool) error {
	t, err := project.GetTask(a.db, id)
	if err != nil {
		return a.fail(1, fmt.Errorf("task show: %w", err))
	}
	if jsonOutput {
		return writeJSON(a.options.Out, t)
	}
	fmt.Fprintf(a.options.Out, "ID: %s\n", t.ID)
	fmt.Fprintf(a.options.Out, "Agent: %s\n", t.AgentType)
	fmt.Fprintf(a.options.Out, "Session: %s\n", t.SessionID)
	fmt.Fprintf(a.options.Out, "Branch: %s\n", t.Branch)
	fmt.Fprintf(a.options.Out, "Directory: %s\n", t.Directory)
	fmt.Fprintf(a.options.Out, "Commit Range: %s\n", t.CommitRange)
	fmt.Fprintf(a.options.Out, "Created: %s\n", t.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(a.options.Out, "Updated: %s\n", t.UpdatedAt.Format(time.RFC3339))
	return nil
}

func (a *app) runTaskUpdate(id string, params project.UpdateTaskParams) error {
	t, err := project.UpdateTask(a.db, id, params)
	if err != nil {
		return a.fail(1, fmt.Errorf("task update: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Updated task %s\n", t.ID)
	return nil
}

func (a *app) runTaskDelete(id string) error {
	if err := project.DeleteTask(a.db, id); err != nil {
		return a.fail(1, fmt.Errorf("task delete: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Deleted task %s\n", id)
	return nil
}

func (a *app) runTaskAttach(taskID, projectName string) error {
	p, err := project.GetProjectByName(a.db, projectName)
	if err != nil {
		return a.fail(1, fmt.Errorf("task attach: %w", err))
	}
	if err := project.AttachTaskToProject(a.db, taskID, p.ID); err != nil {
		return a.fail(1, fmt.Errorf("task attach: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Attached task %s to project %q\n", taskID, projectName)
	return nil
}

func (a *app) runTaskDetach(taskID, projectName string) error {
	p, err := project.GetProjectByName(a.db, projectName)
	if err != nil {
		return a.fail(1, fmt.Errorf("task detach: %w", err))
	}
	if err := project.DetachTaskFromProject(a.db, taskID, p.ID); err != nil {
		return a.fail(1, fmt.Errorf("task detach: %w", err))
	}
	fmt.Fprintf(a.options.Out, "Detached task %s from project %q\n", taskID, projectName)
	return nil
}
