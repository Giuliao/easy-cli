package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Giuliao/easy-cli/internal/projectroot"
)

const contextFileName = "context"

// WorkContext binds a Git worktree to a project and, when known, the task that
// represents this requirement's continuous development in that repository.
// It stores pointers only; branch and commit facts stay in Git and the task.
type WorkContext struct {
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
}

func (c WorkContext) empty() bool {
	return c.ProjectID == "" && c.ProjectName == "" && c.TaskID == ""
}

func contextPath(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve worktree directory %q: %w", directory, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve worktree directory %q; select an existing directory: %w", directory, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect worktree directory %q: %w", canonical, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory; select a worktree directory", canonical)
	}
	root := projectroot.Find(canonical)
	return filepath.Join(root, ".easy-cli", contextFileName), nil
}

// ReadWorkContext returns the current binding for the Git worktree containing
// directory. A legacy file holding only a project name is accepted. A missing
// file means no binding; a damaged file is an error, never absence.
func ReadWorkContext(directory string) (WorkContext, error) {
	path, err := contextPath(directory)
	if err != nil {
		return WorkContext{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return WorkContext{}, nil
	}
	if err != nil {
		return WorkContext{}, fmt.Errorf("read work context %q: %w", path, err)
	}
	ctx, err := parseWorkContext(data)
	if err != nil {
		return WorkContext{}, fmt.Errorf("work context %q: %w", path, err)
	}
	return ctx, nil
}

// WriteWorkContext publishes the binding atomically. It never overwrites a
// damaged context, so a manual typo cannot silently discard the task pointer.
func WriteWorkContext(directory string, ctx WorkContext) error {
	if ctx.ProjectID == "" && ctx.ProjectName == "" {
		return fmt.Errorf("work context requires a project ID or name")
	}
	if ctx.TaskID != "" && ctx.ProjectID == "" && ctx.ProjectName == "" {
		return fmt.Errorf("work context task requires a project")
	}
	path, err := contextPath(directory)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		old, parseErr := parseWorkContext(existing)
		if parseErr != nil {
			return fmt.Errorf("refusing to overwrite damaged work context %q: %w", path, parseErr)
		}
		if old == ctx {
			return nil
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("read work context %q: %w", path, err)
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return fmt.Errorf("encode work context: %w", err)
	}
	return writeContextFile(path, append(data, '\n'))
}

func parseWorkContext(data []byte) (WorkContext, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return WorkContext{}, fmt.Errorf("context file is empty; set the project with easy project use <name>")
	}
	if !strings.HasPrefix(trimmed, "{") {
		// Legacy format: a bare project name. Keep accepting it so an older
		// installation can resume and upgrade on the next ensure.
		if strings.ContainsAny(trimmed, "\r\n") {
			return WorkContext{}, fmt.Errorf("legacy context must contain a single project name")
		}
		return WorkContext{ProjectName: trimmed}, nil
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var ctx WorkContext
	if err := decoder.Decode(&ctx); err != nil {
		return WorkContext{}, fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return WorkContext{}, fmt.Errorf("unexpected data after the context object")
	}
	if ctx.ProjectID == "" && ctx.ProjectName == "" {
		return WorkContext{}, fmt.Errorf("context object requires project_id or project_name")
	}
	if ctx.TaskID != "" && ctx.ProjectID == "" && ctx.ProjectName == "" {
		return WorkContext{}, fmt.Errorf("context task requires a project")
	}
	return ctx, nil
}

func writeContextFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .easy-cli directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".context-*")
	if err != nil {
		return fmt.Errorf("create temporary context file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary context file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("set context permissions: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("flush context file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close context file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Windows cannot replace an existing file with os.Rename. Removing it
		// first is still bounded to the verified context path.
		if runtime.GOOS == "windows" {
			if removeErr := os.Remove(path); removeErr == nil {
				if err := os.Rename(tmpName, path); err == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("publish context file: %w", err)
	}
	return nil
}

// contextSnapshot preserves the exact bytes so a failed database commit can
// restore the previous binding instead of leaving a pointer to an absent task.
type contextSnapshot struct {
	path   string
	data   []byte
	exists bool
}

func snapshotContext(directory string) (contextSnapshot, error) {
	path, err := contextPath(directory)
	if err != nil {
		return contextSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return contextSnapshot{path: path}, nil
	}
	if err != nil {
		return contextSnapshot{}, fmt.Errorf("read work context %q: %w", path, err)
	}
	return contextSnapshot{path: path, data: data, exists: true}, nil
}

func (s contextSnapshot) restore() error {
	if s.exists {
		return writeContextFile(s.path, s.data)
	}
	if filepath.Base(s.path) != contextFileName || filepath.Base(filepath.Dir(s.path)) != ".easy-cli" {
		return fmt.Errorf("refusing to remove unexpected context path %q", s.path)
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("restore absent work context: %w", err)
	}
	return nil
}

// SetCurrentProject writes the legacy plain-name context. It remains for
// compatibility with older installations and is upgraded by work ensure.
func SetCurrentProject(workingDir, name string) error {
	path, err := contextPath(workingDir)
	if err != nil {
		return err
	}
	return writeContextFile(path, []byte(name))
}

// CurrentProject returns the project name from the work context, or "" when
// unset. Callers that need task or project identity must use ReadWorkContext.
func CurrentProject(workingDir string) string {
	ctx, err := ReadWorkContext(workingDir)
	if err != nil {
		return ""
	}
	return ctx.ProjectName
}
