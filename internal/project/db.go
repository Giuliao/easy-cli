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
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			directory TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
			task_selection_required INTEGER NOT NULL DEFAULT 0 CHECK (task_selection_required IN (0, 1)),
			git_common_dir TEXT NOT NULL,
			repository TEXT NOT NULL,
			branch TEXT NOT NULL,
			current_task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workspaces_project_id ON workspaces(project_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_active_directory ON workspaces(directory) WHERE active = 1`,
		`CREATE TABLE IF NOT EXISTS work_task_progress (
			task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
			base_commit TEXT NOT NULL,
			head_commit TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS work_checkpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			task_id TEXT NOT NULL,
			branch TEXT NOT NULL,
			base_commit TEXT NOT NULL,
			head_commit TEXT NOT NULL,
			dirty INTEGER NOT NULL CHECK (dirty IN (0, 1)),
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_work_checkpoints_workspace ON work_checkpoints(workspace_id, id)`,
		`CREATE TRIGGER IF NOT EXISTS clear_detached_current_task
		 AFTER DELETE ON project_tasks
		 BEGIN
			UPDATE workspaces SET current_task_id = NULL, task_selection_required = 1
			WHERE project_id = OLD.project_id AND current_task_id = OLD.task_id;
		 END`,
		`CREATE TRIGGER IF NOT EXISTS require_selection_after_task_delete
		 BEFORE DELETE ON tasks
		 BEGIN
			UPDATE workspaces SET current_task_id = NULL, task_selection_required = 1
			WHERE current_task_id = OLD.id;
		 END`,
		`CREATE TRIGGER IF NOT EXISTS protect_bound_task_location
		 BEFORE UPDATE OF branch, directory ON tasks
		 WHEN EXISTS (SELECT 1 FROM workspaces WHERE current_task_id = OLD.id AND active = 1)
		 AND (NEW.branch != OLD.branch OR NEW.directory != OLD.directory)
		 BEGIN
			SELECT RAISE(ABORT, 'bound task location cannot change; select a different workspace or task');
		 END`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	return nil
}
