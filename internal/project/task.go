package project

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTaskNotFound        = errors.New("task not found")
	ErrTaskAlreadyAttached = errors.New("task already attached to project")
	ErrTaskNotAttached     = errors.New("task not attached to project")
)

type Task struct {
	ID          string    `json:"id"`
	AgentType   string    `json:"agent_type"`
	SessionID   string    `json:"session_id"`
	Branch      string    `json:"branch"`
	Directory   string    `json:"directory"`
	CommitRange string    `json:"commit_range"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateTaskParams struct {
	AgentType   string
	SessionID   string
	Branch      string
	Directory   string
	CommitRange string
}

type UpdateTaskParams struct {
	AgentType   *string
	SessionID   *string
	Branch      *string
	Directory   *string
	CommitRange *string
}

func CreateTask(db *sql.DB, params CreateTaskParams) (Task, error) {
	now := time.Now().UTC()
	t := Task{
		ID:          NewTaskID(),
		AgentType:   params.AgentType,
		SessionID:   params.SessionID,
		Branch:      params.Branch,
		Directory:   params.Directory,
		CommitRange: params.CommitRange,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.Exec(
		`INSERT INTO tasks (id, agent_type, session_id, branch, directory, commit_range, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.AgentType, t.SessionID, t.Branch, t.Directory, t.CommitRange,
		t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Task{}, fmt.Errorf("insert task: %w", err)
	}
	return t, nil
}

func GetTask(db *sql.DB, id string) (Task, error) {
	row := db.QueryRow(
		`SELECT id, agent_type, session_id, branch, directory, commit_range, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	)
	return scanTask(row)
}

func ListTasks(db *sql.DB) ([]Task, error) {
	rows, err := db.Query(
		`SELECT id, agent_type, session_id, branch, directory, commit_range, created_at, updated_at
		 FROM tasks ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func ListTasksByProject(db *sql.DB, projectID string) ([]Task, error) {
	rows, err := db.Query(
		`SELECT t.id, t.agent_type, t.session_id, t.branch, t.directory, t.commit_range, t.created_at, t.updated_at
		 FROM tasks t
		 JOIN project_tasks pt ON pt.task_id = t.id
		 WHERE pt.project_id = ?
		 ORDER BY t.created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("query tasks by project: %w", err)
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func UpdateTask(db *sql.DB, id string, params UpdateTaskParams) (Task, error) {
	existing, err := GetTask(db, id)
	if err != nil {
		return Task{}, err
	}
	if params.AgentType != nil {
		existing.AgentType = *params.AgentType
	}
	if params.SessionID != nil {
		existing.SessionID = *params.SessionID
	}
	if params.Branch != nil {
		existing.Branch = *params.Branch
	}
	if params.Directory != nil {
		existing.Directory = *params.Directory
	}
	if params.CommitRange != nil {
		existing.CommitRange = *params.CommitRange
	}
	existing.UpdatedAt = time.Now().UTC()
	_, err = db.Exec(
		`UPDATE tasks SET agent_type = ?, session_id = ?, branch = ?, directory = ?, commit_range = ?, updated_at = ?
		 WHERE id = ?`,
		existing.AgentType, existing.SessionID, existing.Branch, existing.Directory, existing.CommitRange,
		existing.UpdatedAt.Format(time.RFC3339), id,
	)
	if err != nil {
		return Task{}, fmt.Errorf("update task: %w", err)
	}
	return existing, nil
}

func DeleteTask(db *sql.DB, id string) error {
	result, err := db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %q", ErrTaskNotFound, id)
	}
	return nil
}

func AttachTaskToProject(db *sql.DB, taskID, projectID string) error {
	if _, err := GetTask(db, taskID); err != nil {
		return err
	}
	if _, err := GetProjectByID(db, projectID); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO project_tasks (project_id, task_id) VALUES (?, ?)`,
		projectID, taskID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: task %q is already attached to a project", ErrTaskAlreadyAttached, taskID)
		}
		return fmt.Errorf("attach task: %w", err)
	}
	return nil
}

func DetachTaskFromProject(db *sql.DB, taskID, projectID string) error {
	result, err := db.Exec(
		`DELETE FROM project_tasks WHERE project_id = ? AND task_id = ?`,
		projectID, taskID,
	)
	if err != nil {
		return fmt.Errorf("detach task: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("detach task: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: task %q is not attached to project", ErrTaskNotAttached, taskID)
	}
	return nil
}

func GetProjectForTask(db *sql.DB, taskID string) (Project, error) {
	row := db.QueryRow(
		`SELECT p.id, p.name, p.description, p.created_at, p.status, p.ended_at
		 FROM projects p
		 JOIN project_tasks pt ON pt.project_id = p.id
		 WHERE pt.task_id = ?`, taskID,
	)
	return scanProject(row)
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(s taskScanner) (Task, error) {
	var t Task
	var createdAt, updatedAt string
	if err := s.Scan(&t.ID, &t.AgentType, &t.SessionID, &t.Branch, &t.Directory, &t.CommitRange, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("scan task: %w", err)
	}
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse created_at: %w", err)
	}
	t.CreatedAt = created
	updated, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse updated_at: %w", err)
	}
	t.UpdatedAt = updated
	return t, nil
}
