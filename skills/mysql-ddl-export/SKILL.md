---
name: mysql-ddl-export
description: Export MySQL ordinary-table CREATE TABLE DDL through this repository's easy CLI. Use when the user asks to inspect, snapshot, compare, or retrieve table schema definitions from a MySQL database; this skill does not cover views, routines, triggers, or data.
---

# MySQL DDL Export

Use the repository's `easy mysql ddl` command to retrieve the exact `CREATE TABLE` definitions for a MySQL database.

## Workflow

1. Confirm the target host, port, username, and database. Do not guess a production or shared database when the target is ambiguous.
2. Ensure the CLI exists. From the repository root, run `make build` when `./easy` is not available.
3. Obtain the password through a secure user-approved channel. Prefer stdin so the secret is not placed directly in shell history:

   ```bash
   printf '%s\n' "$MYSQL_PASSWORD" | ./easy mysql ddl \
     --host "$MYSQL_HOST" \
     --port "${MYSQL_PORT:-3306}" \
     --user "$MYSQL_USER" \
     --password-stdin \
     --database "$MYSQL_DATABASE"
   ```

4. Treat stdout as the DDL source of truth. The command sorts ordinary tables by name and separates their `CREATE TABLE` statements with a blank line.
5. If the user asks for a schema review, comparison, or migration analysis, inspect the returned DDL before making claims. Preserve table options, indexes, constraints, character sets, collations, and comments.
6. Report connection or export failures clearly without exposing the password, DSN, or other secrets.

## Scope

The command exports only MySQL `BASE TABLE` definitions. It does not export views, triggers, stored procedures, functions, events, or row data. State this limitation when the user asks for a complete schema dump.

An empty stdout is a successful result when the database has no ordinary tables.

## Safety

- Use the command only for metadata export; do not run write statements or modify the database.
- Never echo, log, commit, or include credentials in the final response.
- Prefer `--password-stdin` over `--password`; never put a real password in an example or generated command.
- Do not retry against a different host or database without user confirmation.
- Do not fabricate DDL when the command fails or returns no tables.
