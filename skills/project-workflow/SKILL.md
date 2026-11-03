---
name: project-workflow
description: Resume and track AI development work by project and Git worktree with easy work commands. Use when starting or continuing work across sessions or agents; restore the existing task before initializing new work.
---

# Project / Worktree Workflow

A **project** groups related work and has status `active` or `ended`. A **task** is the persistent development record of one repository worktree and branch: it spans sessions and agents, and successive feedback, bug fixes and new features continue the same task instead of starting a new one. The worktree's `.easy-cli/context` stores only the `project_id` / `project_name` / `task_id` pointers; directory, branch, Git common dir and commit range always come from the real Git state. Linked worktrees keep independent context files even when they share a Git common dir.

## Operating rules

- Resume before initializing. A new session, a different agent, a bug fix or an added feature does not create a new task, branch or worktree. Stay in the existing directory and branch unless the user explicitly requests a different setup.
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

From a generic directory, first find the recorded projects and tasks:

```bash
easy project list
easy project show <name> --json
```

`project show` lists the project's tasks with directory, branch and commit range. Confirm the directory and branch that match the real Git worktree before resuming, then use that directory rather than the generic startup directory.

`status` is read-only; an unbound worktree reports an actionable error. `ensure` reuses the current task, and `--agent` only supplies metadata on initial creation. Without a task pointer, it reuses the unique task matching project/directory/branch; several candidates require `--task <id>`, never a guess. Only when no task exists for that directory does it create one, with empty session metadata and the initial HEAD as base. A branch that differs from the recorded task, a stale pointer or a pointer to another project is reported as drift and refused; nothing is repaired automatically. To clear a stale pointer, run `project use <name> --replace` and ensure again.

### 2. Initialize only when no existing work applies

Confirm the project and worktree. Create a project only for a genuinely new effort, then bind the worktree and ensure its task:

```bash
easy project create <name> --description "<goal>"
easy project use <name> --dir <worktree>
easy work ensure --project <name> --dir <worktree> --agent <type>
```

`project use` does not create a task; repeating it for the same project is idempotent and keeps the current task. `ensure --project <name>` can bind an unbound worktree. A worktree already bound to a different project requires the user's decision before `project use <name> --dir <worktree> --replace`: the context is re-pointed and the task pointer cleared, while the old project, tasks and checkpoints stay untouched. The next `ensure` creates that project's own task for this worktree.

Several repositories in one requirement are several tasks, one per worktree. To select a known compatible task, name it explicitly:

```bash
easy work ensure --project <name> --dir <worktree> --task <id>
```

There is no `--new-task` flag. A new task is created only when the worktree genuinely has no record, or when the user explicitly asks for a separate record (then create it with `easy task create` and select it with `--task`). Separate worktrees for parallel work require an explicit user choice; each agent then binds and resumes its own approved worktree.

### 3. Work and checkpoint

Develop within the user's scope; do not commit merely for tracking. At useful milestones and before handoff, collect progress and inspect history:

```bash
easy work checkpoint --dir <worktree> --json
easy work history --dir <worktree> --json
```

`checkpoint` requires an existing current task; it never calls ensure/create. The initial base stays fixed across sessions, head advances to current HEAD, and immutable history records dirty state (staged, unstaged and non-ignored untracked files). Nothing is committed or changed in Git. `history` returns the whole checkpoint history of the current task, including entries recorded from earlier sessions and agents. To read a task's history without the worktree being online, use `easy work history --task <id> --json` with the ID from `project show`.

The old head must remain an ancestor of current HEAD. Branch/common-dir/association drift, invalid ranges or reset/rebase divergence cause refusal, not silent repair. Run `status`, explain the mismatch and ask the user; restore the correct branch or binding, or explicitly select a different task, only after review. Legacy empty ranges start at current HEAD and cannot recover previously unrecorded history. A commit range describes the task's development record, not an agent's exclusive contribution.

`easy task create` / `update` remain available for explicit record management; `--session` is optional compatibility metadata and never drives recovery. Explicit task creation does not bind the worktree; select it with `work ensure --task`. Do not use manual range updates to bypass checkpoint validation.

### 4. Handoff without closing

Checkpoint and report project/task, directory/branch, commit range, dirty state, validation and remaining work. Keep the project active while requirements may continue across sessions.

Only when the user explicitly requests the lifecycle change:

```bash
easy project close <name> --confirm
easy project reopen <name> --confirm
```

Closing sets the project to `ended`; tasks and checkpoint history are retained. The flag does not establish that the user approved the action. A resumed session must not close a project merely because its own turn ended.
