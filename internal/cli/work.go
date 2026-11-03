package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Giuliao/easy-cli/internal/project"
	"github.com/spf13/cobra"
)

func (a *app) workDirectory(directory string) string {
	if directory == "" {
		return a.options.WorkingDir
	}
	if !filepath.IsAbs(directory) && a.options.WorkingDir != "" {
		return filepath.Join(a.options.WorkingDir, directory)
	}
	return directory
}

func (a *app) buildWorkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "work", Short: "Resume project work by worktree, independent of agent sessions.",
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	for _, action := range []string{"status", "ensure", "checkpoint", "history"} {
		var directory string
		var jsonOutput bool
		var workspaceID string
		var params project.EnsureWorkParams
		sub := &cobra.Command{
			Use: action, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				dir := a.workDirectory(directory)
				if action == "history" {
					var history []project.Checkpoint
					var err error
					if workspaceID != "" {
						history, err = project.WorkHistoryByWorkspace(a.db, workspaceID)
					} else {
						history, err = project.WorkHistory(a.db, dir)
					}
					if err != nil {
						return a.fail(1, fmt.Errorf("work history: %w", err))
					}
					if jsonOutput {
						return writeJSON(a.options.Out, history)
					}
					rows := make([][]string, 0, len(history))
					for _, cp := range history {
						rows = append(rows, []string{fmt.Sprint(cp.ID), cp.TaskID, cp.Branch, cp.BaseCommit + ".." + cp.HeadCommit, fmt.Sprint(cp.Dirty), cp.CreatedAt.Format(time.RFC3339Nano)})
					}
					printTable(a.options.Out, []string{"ID", "TASK", "BRANCH", "COMMIT_RANGE", "DIRTY", "CREATED"}, rows)
					return nil
				}
				var state project.WorkState
				var err error
				switch action {
				case "status":
					state, err = project.CurrentWork(a.db, dir)
				case "ensure":
					params.Directory = dir
					state, err = project.EnsureWork(a.db, params)
				case "checkpoint":
					state, err = project.CheckpointWork(a.db, dir)
				}
				if err != nil {
					return a.fail(1, fmt.Errorf("work %s: %w", action, err))
				}
				if jsonOutput {
					return writeJSON(a.options.Out, state)
				}
				a.printWorkState(state)
				return nil
			},
		}
		sub.Flags().StringVar(&directory, "dir", "", "worktree directory (defaults to current directory)")
		sub.Flags().BoolVar(&jsonOutput, "json", false, "output JSON")
		switch action {
		case "status":
			sub.Short = "Show current project, task and actual Git state without modifying records."
		case "ensure":
			sub.Short = "Restore or initialize the current work segment without creating a branch."
			sub.Flags().StringVar(&params.ProjectName, "project", "", "explicit project to resume (must match existing binding)")
			sub.Flags().StringVar(&params.AgentType, "agent", "", "agent type for a newly created task only")
			sub.Flags().StringVar(&params.TaskID, "task", "", "explicitly select an existing task in this workspace")
			sub.Flags().BoolVar(&params.NewTask, "new-task", false, "explicitly start a new work segment in the same workspace")
			sub.MarkFlagsMutuallyExclusive("task", "new-task")
		case "checkpoint":
			sub.Short = "Record Git HEAD and dirty state, preserving the task's original base."
		case "history":
			sub.Short = "List saved Git checkpoints for this workspace."
			sub.Flags().StringVar(&workspaceID, "workspace", "", "explicit workspace ID, including archived/offline workspaces")
			sub.MarkFlagsMutuallyExclusive("dir", "workspace")
		}
		cmd.AddCommand(sub)
	}
	return cmd
}

func (a *app) printWorkState(s project.WorkState) {
	fmt.Fprintf(a.options.Out, "Project: %s (%s)\n", s.Project.Name, s.Project.Status)
	fmt.Fprintf(a.options.Out, "Workspace: %s\nRepository: %s\nDirectory: %s\nBranch: %s\n", s.Workspace.ID, s.Workspace.Repository, s.Workspace.Directory, s.Workspace.Branch)
	if s.Task != nil {
		fmt.Fprintf(a.options.Out, "Task: %s\nCommit Range: %s\n", s.Task.ID, s.Task.CommitRange)
	} else {
		fmt.Fprintln(a.options.Out, "Task: (not selected; run easy work ensure)")
	}
	fmt.Fprintf(a.options.Out, "Git Branch: %s\nHEAD: %s\nDirty: %t\n", s.Git.Branch, s.Git.Head, s.Git.Dirty)
	if s.Created {
		fmt.Fprintln(a.options.Out, "Created a new work segment; later sessions will reuse this task.")
	}
	if s.Checkpoint != nil {
		fmt.Fprintf(a.options.Out, "Checkpoint: %d\n", s.Checkpoint.ID)
	}
	for _, issue := range s.Issues {
		fmt.Fprintf(a.options.Out, "Issue: %s\n", issue)
	}
}
