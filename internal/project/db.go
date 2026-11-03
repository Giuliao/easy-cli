package project

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const dbFileName = "easy.db"

// Open opens (or creates) the SQLite database at the default location under
// the user's home configuration directory and ensures the schema exists.
func Open(homeDir string) (*sql.DB, error) {
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
	}
	dir := filepath.Join(homeDir, ".config", "easy-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	path := filepath.Join(dir, dbFileName)
	// Apply connection-local pragmas to every connection, not just the first.
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String() +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			status      TEXT NOT NULL CHECK (status IN ('active', 'ended')),
			ended_at    TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id           TEXT PRIMARY KEY,
			agent_type   TEXT NOT NULL,
			session_id   TEXT NOT NULL,
			branch       TEXT NOT NULL,
			directory    TEXT NOT NULL,
			commit_range TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS project_tasks (
			project_id TEXT NOT NULL,
			task_id    TEXT NOT NULL UNIQUE,
			PRIMARY KEY (project_id, task_id),
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_project_tasks_project_id ON project_tasks(project_id)`,
		`CREATE TABLE IF NOT EXISTS work_task_progress (
			task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
			base_commit TEXT NOT NULL,
			head_commit TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS task_checkpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			branch TEXT NOT NULL,
			base_commit TEXT NOT NULL,
			head_commit TEXT NOT NULL,
			dirty INTEGER NOT NULL CHECK (dirty IN (0, 1)),
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_checkpoints_task ON task_checkpoints(task_id, id)`,
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin database migration: %w", err)
	}
	defer tx.Rollback()
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	// Preserve checkpoints created by the unreleased workspace-based PR, then
	// remove its abandoned schema. No local context files are modified here.
	var oldCheckpoints int
	if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'work_checkpoints'`).Scan(&oldCheckpoints); err != nil {
		return err
	}
	if oldCheckpoints != 0 {
		if _, err := tx.Exec(`INSERT INTO task_checkpoints (task_id, branch, base_commit, head_commit, dirty, created_at)
			SELECT cp.task_id, cp.branch, cp.base_commit, cp.head_commit, cp.dirty, cp.created_at
			FROM work_checkpoints cp JOIN tasks t ON t.id = cp.task_id ORDER BY cp.id`); err != nil {
			return fmt.Errorf("migrate task checkpoints: %w", err)
		}
	}
	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS clear_detached_current_task`,
		`DROP TRIGGER IF EXISTS require_selection_after_task_delete`,
		`DROP TRIGGER IF EXISTS protect_bound_task_location`,
		`DROP TABLE IF EXISTS work_checkpoints`,
		`DROP TABLE IF EXISTS workspaces`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("remove superseded PR schema: %w", err)
		}
	}
	return tx.Commit()
}
