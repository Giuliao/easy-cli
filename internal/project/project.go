package project

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StatusActive = "active"
	StatusEnded  = "ended"
)

var (
	ErrProjectNotFound      = errors.New("project not found")
	ErrProjectAlreadyExists = errors.New("project already exists")
)

type Project struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	Status      string     `json:"status"`
	EndedAt     *time.Time `json:"ended_at"`
}

type CreateProjectParams struct {
	Name        string
	Description string
}

func CreateProject(db *sql.DB, params CreateProjectParams) (Project, error) {
	now := time.Now().UTC()
	p := Project{
		ID:          NewProjectID(),
		Name:        params.Name,
		Description: params.Description,
		CreatedAt:   now,
		Status:      StatusActive,
	}
	_, err := db.Exec(
		`INSERT INTO projects (id, name, description, created_at, status, ended_at)
		 VALUES (?, ?, ?, ?, ?, NULL)`,
		p.ID, p.Name, p.Description, p.CreatedAt.Format(time.RFC3339), p.Status,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Project{}, fmt.Errorf("%w: %q", ErrProjectAlreadyExists, params.Name)
		}
		return Project{}, fmt.Errorf("insert project: %w", err)
	}
	return p, nil
}

func GetProjectByName(db *sql.DB, name string) (Project, error) {
	row := db.QueryRow(
		`SELECT id, name, description, created_at, status, ended_at
		 FROM projects WHERE name = ?`, name,
	)
	return scanProject(row)
}

func GetProjectByID(db *sql.DB, id string) (Project, error) {
	row := db.QueryRow(
		`SELECT id, name, description, created_at, status, ended_at
		 FROM projects WHERE id = ?`, id,
	)
	return scanProject(row)
}

func ListProjects(db *sql.DB) ([]Project, error) {
	rows, err := db.Query(
		`SELECT id, name, description, created_at, status, ended_at
		 FROM projects ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func CloseProject(db *sql.DB, name string) (Project, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(
		`UPDATE projects SET status = ?, ended_at = ? WHERE name = ?`,
		StatusEnded, now, name,
	)
	if err != nil {
		return Project{}, fmt.Errorf("close project: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Project{}, fmt.Errorf("close project: %w", err)
	}
	if affected == 0 {
		return Project{}, fmt.Errorf("%w: %q", ErrProjectNotFound, name)
	}
	return GetProjectByName(db, name)
}

func DeleteProject(db *sql.DB, name string) error {
	result, err := db.Exec(`DELETE FROM projects WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: %q", ErrProjectNotFound, name)
	}
	return nil
}

type projectScanner interface {
	Scan(dest ...any) error
}

func scanProject(s projectScanner) (Project, error) {
	var p Project
	var createdAt string
	var endedAt sql.NullString
	if err := s.Scan(&p.ID, &p.Name, &p.Description, &createdAt, &p.Status, &endedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("scan project: %w", err)
	}
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Project{}, fmt.Errorf("parse created_at: %w", err)
	}
	p.CreatedAt = created
	if endedAt.Valid {
		ended, err := time.Parse(time.RFC3339, endedAt.String)
		if err != nil {
			return Project{}, fmt.Errorf("parse ended_at: %w", err)
		}
		p.EndedAt = &ended
	}
	return p, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
