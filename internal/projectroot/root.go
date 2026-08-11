package projectroot

import (
	"os"
	"path/filepath"
)

// Find returns the nearest ancestor containing a .git entry. If none exists,
// it returns start so callers can still use a directory outside a Git repo.
func Find(start string) string {
	current, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	original := current
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return original
		}
		current = parent
	}
}
