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
| Inspect one skill | `easy skill show <skill-name>` | Read metadata and installation status |

For MySQL DDL requests, first load `mysql-ddl-export` when its workflow is needed, then execute `easy mysql ddl` with the user-confirmed connection details. Do not treat reading a prompt as completing the database export.

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
