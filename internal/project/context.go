package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Giuliao/easy-cli/internal/projectroot"
)

const contextFileName = "context"

// SetCurrentProject writes the project name into the .easy-cli/context file
// located at the root of the git repository that contains workingDir.
func SetCurrentProject(workingDir, name string) error {
	root := projectroot.Find(workingDir)
	dir := filepath.Join(root, ".easy-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .easy-cli directory: %w", err)
	}
	path := filepath.Join(dir, contextFileName)
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		return fmt.Errorf("write context file: %w", err)
	}
	return nil
}

// CurrentProject returns the project name stored in the .easy-cli/context
// file at the root of the git repository that contains workingDir.
// It returns an empty string if no context is set.
func CurrentProject(workingDir string) string {
	root := projectroot.Find(workingDir)
	path := filepath.Join(root, ".easy-cli", contextFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
