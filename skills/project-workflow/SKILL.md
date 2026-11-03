---
name: project-workflow
description: Resume and track AI development work by project and Git worktree with easy work commands. Use when starting or continuing work across sessions or agents; restore the existing workspace before initializing new work.
---

# Project / Worktree Workflow

A **project** groups related work and has status `active` or `ended`. A **workspace** binds a project to a real Git worktree (directory, Git common dir, repository and branch). A **task** is a work segment that can span sessions and agents, not a session identity. Each workspace has an optional current task; linked worktrees have independent bindings even when they share a Git common dir.

## Operating rules

- Resume before initializing. A new session or agent does not create a new task, branch or worktree. Stay in the existing directory and branch unless the user explicitly requests a different setup.
- Do not select a project/task by session ID, recent activity or a similar name. `--session` is optional legacy metadata: do not generate or require it, and do not use it for recovery.
- Ask the user about binding conflicts, multiple task candidates, branch/history drift or a closed project. Do not silently replace bindings, rewrite ranges or reopen projects.
- Session completion is not project completion. Never close/reopen a project without an explicit human request; `--confirm` is an accident guard, not proof of authorization.
- These CLI commands are externally callable primitives. No agent hooks are integrated, and this SOP cannot guarantee invocation on every turn.

## SOP

### 1. Locate and resume the existing work

Read the repository's development instructions first. In a known worktree, inspect its binding and actual Git state, then restore idempotently:

```bash
easy work status --dir <worktree> --json
easy work ensure --dir <worktree> --agent <type> --json
```

From a generic directory, first find the recorded workspaces and tasks:

```bash
easy project list
easy project show <name> --json
```

`project show` lists tasks and workspaces with an `active` flag, including archived historical locations. Confirm the active workspace before resuming; it does not verify that other workspaces are online. Use the confirmed directory, not the generic startup directory.

`status` is read-only; an unbound workspace reports an actionable error. `ensure` reuses the current task, and `--agent` only supplies metadata on initial creation. Without a current task, it reuses the unique old task matching project/directory/branch; multiple candidates require `--task <id>` or an explicit `--new-task`, not a guess. With no candidate it creates a task with empty session metadata and the initial HEAD as base. After replacement, deletion or detachment clears the current task, explicitly select `--task` or `--new-task`; never inherit an older task automatically. Binding, lifecycle, branch or history inconsistencies must be resolved before continuing.

### 2. Initialize only when no existing work applies

Confirm the project and worktree. Create a project only for a genuinely new effort, then bind the existing workspace and ensure a task:

```bash
easy project create <name> --description "<goal>"
easy project use <name> --dir <worktree>
easy work ensure --project <name> --dir <worktree> --agent <type>
```

`project use` does not create a task and is idempotent for the same binding. `ensure --project <name>` can establish an unbound workspace; old context may also guide first binding, but the database binding takes priority. A conflicting project/branch/common-dir binding requires the user's decision before `project use <name> --dir <worktree> --replace`: archive the old workspace and its history, create a new workspace identity, and transfer neither the old task nor old-project checkpoints. Explicit `--replace` on the same binding may clear current task, but never rewrites history.

To select a known compatible task or deliberately start another work segment in the same worktree, use exactly one option:

```bash
easy work ensure --project <name> --dir <worktree> --task <id>
easy work ensure --project <name> --dir <worktree> --agent <type> --new-task
```

`--new-task` does not create a branch. Separate worktrees for parallel work require an explicit user choice; each agent then binds and resumes its own approved workspace.

### 3. Work and checkpoint

Develop within the user's scope; do not commit merely for tracking. At useful milestones and before handoff, collect progress and inspect history:

```bash
easy work checkpoint --dir <worktree> --json
easy work history --dir <worktree> --json
```

`checkpoint` requires an existing current task; it never calls ensure/create. The initial base stays fixed across sessions, head advances to current HEAD, and immutable history records dirty state (staged, unstaged and non-ignored untracked files). Nothing is committed or changed in Git. `history` includes earlier tasks in this workspace. To read an archived/offline workspace's history, use `easy work history --workspace <id> --json` with the ID from `project show`; do not mix another project's checkpoints.

The old head must remain an ancestor of current HEAD. Branch/common-dir/association drift, invalid ranges or reset/rebase divergence cause refusal, not silent repair. Run `status`, explain the mismatch and ask the user; restore the correct branch/binding or explicitly choose a new work segment only after review. Legacy empty ranges start at current HEAD and cannot recover previously unrecorded history. Ranges describe a work segment, not an agent's exclusive contribution.

`easy task create` / `update` remain available for explicit record management; `--session` is optional compatibility metadata. Explicit task creation does not set the workspace's current task. Do not use manual range updates to bypass checkpoint validation.

### 4. Handoff without closing

Checkpoint and report project/task, directory/branch, commit range, dirty state, validation and remaining work. Keep the project active while requirements may continue across sessions.

Only when the user explicitly requests the lifecycle change:

```bash
easy project close <name> --confirm
easy project reopen <name> --confirm
```

Closing sets the project to `ended`; tasks are retained. The flag does not establish that the user approved the action.
