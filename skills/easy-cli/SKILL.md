---
name: easy-cli
description: Use the easy CLI to discover and invoke repository skills and built-in capabilities; use installation or update commands only when the user explicitly asks to manage skills.
---

# easy-cli

Use easy CLI as a capability router and command reference. When a user asks for work covered by another skill, load that skill's prompt or invoke the matching built-in command through easy CLI.

## Operating rule

Prioritize using capabilities over managing skills:

- Do not install or update a skill merely because it matches the user's task.
- 不要因为识别到某个 skill 就执行 install 或 update。
- Only run `easy skill install` or `easy skill update` when the user explicitly asks to install, update, or manage a skill.
- Treat the output of `easy skill prompt <skill-name>` as the active constraints for the current task.

## Discover capabilities

List the available capabilities before guessing a skill name:

    easy skill list

Use the following command to inspect one skill's purpose and installation status:

    easy skill show <skill-name>

## Invoke another skill

1. Select the skill from `easy skill list` and confirm its description matches the task.
2. Load its compressed prompt:

       easy skill prompt <skill-name>

3. Follow the returned prompt as task-local constraints.

The same operation has a shorthand command:

    easy <skill-name>

The shorthand is equivalent to `easy skill prompt <skill-name>` and outputs the compressed prompt to stdout. It does not install or activate a skill automatically.

## Capability map

| User intent | Command | Use |
| --- | --- | --- |
| Discover capabilities | `easy skill list` | List skill names, descriptions, and installation status |
| Read SMB development constraints | `easy skill prompt smb-work-order` | Load the local development and delivery rules |
| Read MySQL DDL workflow | `easy skill prompt mysql-ddl-export` | Load connection, password, safety, and result-handling rules |
| Read project/worktree workflow | `easy skill prompt project-workflow` | Load the resume-first SOP for work segments spanning sessions and agents |
| Inspect current work | `easy work status [--dir <path>] [--json]` | Read the workspace binding, current task and actual Git state without writing |
| Resume work idempotently | `easy work ensure [--project <name>] [--dir <path>] [--agent <type>] [--task <id> \| --new-task] [--json]` | Restore the existing task; create only when no candidate exists or a new work segment is explicit |
| Record work progress | `easy work checkpoint [--dir <path>] [--json]` | Advance head with fixed base and record dirty state/history; no task creation or Git mutation |
| Inspect recorded progress | `easy work history [--dir <path> \| --workspace <id>] [--json]` | List workspace checkpoints, including earlier tasks or archived/offline workspaces |
| Export MySQL table DDL | `easy mysql ddl ...` | Execute the read-only DDL export |
| Query MySQL data | `easy mysql query --sql <statement>` | Execute the supplied SQL and return JSON rows by default |
| Locate project workspaces | `easy project show <name> [--json]` | Show tasks and bound workspaces, including when starting in a generic directory |
| Bind a worktree | `easy project use <name> [--dir <path>] [--replace]` | Bind without creating a task; replacement requires the user's decision |
| Manage project lifecycle | `easy project <subcommand>` | Create/list/delete records; close/reopen require an explicit human request and `--confirm` |
| Manage task records | `easy task <subcommand>` | Explicit CRUD and attach/detach; optional legacy `--session` is not recovery identity |
| Initialize Home configuration | `easy config init` | Create a private local template when the user explicitly requests setup |
| Read a configured value | `easy config get <key>` | Read one allowed non-sensitive Home/project configuration value |
| Inspect one skill | `easy skill show <skill-name>` | Read metadata and installation status |

easy also discovers skills from `~/.config/easy-cli/skills/<name>/SKILL.md` and the current project's `.easy-cli/skills/<name>/SKILL.md`. Project skills override Home skills with the same name. Read the catalog below to find external capabilities, then load only the selected skill with `easy skill prompt <name>`.

<!-- EASY_EXTERNAL_SKILLS_START -->
<!-- EASY_EXTERNAL_SKILLS_END -->

## Development work routing

For development work, load `project-workflow` and resume the known project/worktree before initializing anything. A task is a cross-session work segment; changing session or agent does not create a task, branch or worktree. Do not generate or require `--session`.

`work ensure` returns the existing current task idempotently; `--agent` only applies on creation. Without a current task, reuse the unique matching old task; multiple candidates require an explicit `--task <id>` or `--new-task`. After replacement, deletion or detachment clears the current task, require an explicit `--task` or `--new-task` instead of inheriting an older task. Binding conflicts require the user before `project use --replace`. When project/branch/common dir changes, replacement archives the old workspace and history, creates a new workspace identity, and transfers neither old task nor old-project checkpoints. Reusing the same binding is idempotent; explicit `--replace` on that binding may clear current task without rewriting history. `project show` includes `active` flags and archived locations; confirm the active workspace, as it does not check whether other workspaces are online.

Checkpoint keeps base stable, advances head and records dirty state without committing. Divergent history, branch/common-dir/association drift or a closed project must not be silently repaired. A session's completion never authorizes `project close <name> --confirm` or `project reopen <name> --confirm`: require the user's explicit request; the flag is only an accident guard.

Status / ensure / checkpoint / history are callable by external agents or hosts; no specific agent hook is integrated and no every-turn execution is guaranteed.

## Configuration

easy loads `~/.config/easy-cli/config.json` first, then `<project-root>/.easy-cli/config.json`; fields in the project file override Home fields. Explicit MySQL flags override both configuration files. MySQL connection fields can therefore be omitted when they are already configured, but `mysql query --sql <statement>` remains required.

Only when the user explicitly asks to create or reset Home configuration, run `easy config init`; it creates a private template and refuses to overwrite an existing file unless the user explicitly requests `easy config init --force`. Do not initialize configuration merely because a value is missing.

Use `easy config get <key>` only for allowed non-sensitive values. `mysql.password` is intentionally unavailable from this command. Do not print or request a configured password merely to inspect configuration.

For SMB work-order tasks, load `smb-work-order` first and follow its repository resolution instructions.

For MySQL DDL requests, first load `mysql-ddl-export` when its workflow is needed, then execute `easy mysql ddl` with the user-confirmed connection details or configured defaults. Do not treat reading a prompt as completing the database export.

For MySQL data requests, execute `easy mysql query` with the confirmed connection details and the user's SQL. Configured connection fields can be omitted:

    easy mysql query \
      --password-stdin \
      --sql "$MYSQL_SQL"

The query command sends the SQL as provided and does not restrict it to SELECT or other read-only statements. Confirm the target database and the possible write effect before running it. Use `--format table` for terminal-oriented output; JSON is the default for Agent processing.

Use password stdin when possible:

    printf '%s\n' "$MYSQL_PASSWORD" | easy mysql ddl \
      --host "$MYSQL_HOST" \
      --port "${MYSQL_PORT:-3306}" \
      --user "$MYSQL_USER" \
      --password-stdin \
      --database "$MYSQL_DATABASE"

## Manage skill installation and updates

Enter this section only when the user explicitly asks to install or update a skill.

Install the requested skill into the current project:

    easy skill install <skill-name>

Use `--global` for the user-level installation or `--force` to overwrite a different existing file:

    easy skill install <skill-name> --global
    easy skill install <skill-name> --force

Update an already installed skill from the version registered by the current easy binary and local skill directories:

    easy skill update <skill-name>
    easy skill update <skill-name> --global

`update` only updates an existing installation. If the target is absent, install it first. Installing `easy-cli` installs only this aggregate skill; it does not install `mysql-ddl-export` or `smb-work-order`.
