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
| Export MySQL table DDL | `easy mysql ddl ...` | Execute the read-only DDL export |
| Query MySQL data | `easy mysql query --sql <statement>` | Execute the supplied SQL and return JSON rows by default |
| Initialize Home configuration | `easy config init` | Create a private local template when the user explicitly requests setup |
| Read a configured value | `easy config get <key>` | Read one allowed non-sensitive Home/project configuration value |
| Inspect one skill | `easy skill show <skill-name>` | Read metadata and installation status |

## Configuration

easy loads `~/.config/easy-cli/config.json` first, then `<project-root>/.easy-cli/config.json`; fields in the project file override Home fields. Explicit MySQL flags override both configuration files. MySQL connection fields can therefore be omitted when they are already configured, but `mysql query --sql <statement>` remains required.

Only when the user explicitly asks to create or reset Home configuration, run `easy config init`; it creates a private template and refuses to overwrite an existing file unless the user explicitly requests `easy config init --force`. Do not initialize configuration merely because a value is missing.

Use `easy config get <key>` only for allowed non-sensitive values, especially SMB repository paths. `mysql.password` is intentionally unavailable from this command. Do not print or request a configured password merely to inspect configuration.

For SMB work-order tasks, load `smb-work-order` first, then resolve only the repository needed for the request with `easy config get smb.backend-repo`, `easy config get smb.frontend-repo`, or `easy config get smb.idl-repo`.

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

Update an already installed skill from the version embedded in the current easy binary:

    easy skill update <skill-name>
    easy skill update <skill-name> --global

`update` only updates an existing installation. If the target is absent, install it first. Installing `easy-cli` installs only this aggregate skill; it does not install `mysql-ddl-export` or `smb-work-order`.
