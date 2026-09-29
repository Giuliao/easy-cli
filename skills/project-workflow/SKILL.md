---
name: project-workflow
description: Use easy project and task commands to track AI agent development work. Use when starting or continuing a development task that may involve one or more agent sessions, especially when parallel work happens across git worktrees.
---

# Project & Task Workflow

`easy project` and `easy task` let you track development work across one or more AI agent sessions. A **project** groups related work; a **task** records a single agent session's branch, directory, and commit range.

## When to use this workflow

Use it whenever you are about to do development work, especially when:

- The work is large enough to be split across multiple agent sessions.
- Another agent may work on the same project in a separate git worktree.
- You want a record of which agent worked on what, on which branch, and over which commit range.

## Core concepts

- **Project**: a container for related tasks. Has a name, description, and status (`active` or `ended`).
- **Task**: a record of one agent session. Stores the agent type, session ID, branch, working directory, and current commit range. A task belongs to at most one project.
- **Context**: the current project for a git repository, stored in `.easy-cli/context`. Once set, new tasks in that repo attach to it automatically.

## Standard operating procedure

### 1. Create or select the project

Before starting work, make sure a project exists for the effort.

List existing projects:

    easy project list

If the project already exists, note its name. Otherwise create it:

    easy project create <name> --description "<what this project is about>"

If you are working inside the project's git repository, set it as the current project so tasks attach automatically:

    easy project use <name>

### 2. Set up the workspace

Before creating a task, prepare your working branch and directory.

**Default behavior**: create a new branch in a separate git worktree, based on the repository's default branch (`main`, or `master` if `main` does not exist). This isolates your work from the user's current working tree and avoids disturbing their state.

#### Detect the default branch

```
git rev-parse --verify main >/dev/null 2>&1 && BASE=main || BASE=master
```

Use `$BASE` as the base branch unless the user specifies otherwise.

#### Interpret user overrides

The user may change the default behavior. Look for these intents in their request:

| User intent (examples) | Use worktree? | Branch action |
|---|---|---|
| (not specified) | yes | create a new branch from `$BASE` |
| "no worktree", "in current directory", "don't create worktree" | no | create a new branch from `$BASE` in the current directory |
| "use branch X", "on branch X" | yes | check out existing branch `X` |
| "use branch X, no worktree" | no | check out existing branch `X` |
| "base on Y", "from Y", "branch off Y" | yes | create a new branch from `Y` |
| "new branch X from Y" | yes | create new branch `X` from `Y` |

If the user gives a branch name, use it. Otherwise derive one from the project name or a short description of the work, e.g. `feature/<project>-<short-desc>`.

#### Commands

New worktree + new branch from `$BASE` (the default):

```
git worktree add ../<worktree-dir> -b <new-branch> $BASE
cd ../<worktree-dir>
```

No worktree + new branch from `$BASE`:

```
git checkout -b <new-branch> $BASE
```

Use an existing branch with a worktree:

```
git worktree add ../<worktree-dir> <existing-branch>
cd ../<worktree-dir>
```

Use an existing branch without a worktree:

```
git checkout <existing-branch>
```

Name the worktree directory after the branch, e.g. `../wt-<branch>`. After this step, your current directory and branch are the ones recorded in the task.

### 3. Create a task for your session

At the start of your agent session, create a task that records who you are and where you are working.

    easy task create \
      --agent <your-agent-type> \
      --session <your-session-id> \
      --branch <current-branch> \
      --dir <working-directory> \
      [--commit-range <from>..<to>] \
      [--project <project-name>]

Field guidance:

- `--agent`: the agent type, e.g. `claude`, `codex`, `cursor`. Free text.
- `--session`: a stable identifier for this agent session. Use whatever session ID your agent tool provides; if none is available, generate one and keep it for the lifetime of this session.
- `--branch`: the git branch you are working on.
- `--dir`: the working directory (usually the repository or worktree root).
- `--commit-range`: optional. The range of commits this session has produced, e.g. `abc123..def456`. Omit it if you have not committed yet.
- `--project`: optional if `easy project use` was already run in this repo; otherwise specify the project name.

The command prints the new task ID. **Keep it** — you will need it to update the task later.

### 4. Do the work

Develop normally. Commit on your branch. If you are working in a separate git worktree, that is fine — each worktree is its own directory and branch, so create one task per worktree/session.

### 5. Update the commit range as you go

Whenever you make new commits, update the task's commit range so the project record reflects your progress.

    easy task update <task-id> --commit-range <from>..<to>

You can also update other fields if they change (branch, session, directory, agent).

### 6. Check what others are doing

To see all tasks under the project (including those from other agents in other worktrees):

    easy project show <project-name>

Or list tasks filtered by project:

    easy task list --project <project-name>

This helps you avoid conflicts and understand what parallel work is in flight.

### 7. Finish up

When your session's work is complete:

1. Update the commit range one final time:

       easy task update <task-id> --commit-range <from>..<to>

2. If the whole project is done, close it:

       easy project close <project-name>

Closing a project sets its status to `ended` and records the end time. Tasks are not deleted.

## Parallel work with git worktrees

When multiple agents work on the same project simultaneously:

1. One agent creates the project: `easy project create <name>`.
2. Each agent creates its own worktree and its own task:

       git worktree add ../worktree-1 -b feature/foo
       cd ../worktree-1
       easy project use <name>
       easy task create --agent claude --session s1 --branch feature/foo --dir "$PWD"

3. Agents update their own tasks as they commit.
4. Any agent can run `easy project show <name>` to see all tasks and their commit ranges.

Because each task records its own branch and directory, there is no ambiguity about who worked where.

## Tips

- Run `easy task create` early in the session, before you forget the session ID.
- If you do not know the session ID, use a memorable string you can recognize later.
- Keep commit ranges in `from..to` form so they are directly usable with `git log`.
- Use `--json` on any list/show command when you need machine-readable output.
