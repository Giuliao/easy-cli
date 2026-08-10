package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytedance/easy-cli/internal/prompt"
)

type InstallOptions struct {
	Global     bool
	Force      bool
	WorkingDir string
	HomeDir    string
}

type InstallResult struct {
	Path    string
	Changed bool
}

func Install(skill Skill, options InstallOptions) (InstallResult, error) {
	return write(skill, options, false)
}

func Update(skill Skill, options InstallOptions) (InstallResult, error) {
	options.Force = true
	return write(skill, options, true)
}

func write(selected Skill, options InstallOptions, requireExisting bool) (InstallResult, error) {
	targetPath, err := InstallPath(selected.Name, options)
	if err != nil {
		return InstallResult{}, err
	}
	targetDir := filepath.Dir(targetPath)

	content, err := prompt.Compress(selected.Source)
	if err != nil {
		return InstallResult{}, fmt.Errorf("compress skill %q: %w", selected.Name, err)
	}
	if existing, readErr := os.ReadFile(targetPath); readErr == nil {
		if string(existing) == content {
			return InstallResult{Path: targetPath}, nil
		}
		if !options.Force {
			return InstallResult{}, fmt.Errorf("target exists with different content: %s", targetPath)
		}
	} else if !os.IsNotExist(readErr) {
		return InstallResult{}, fmt.Errorf("read target %s: %w", targetPath, readErr)
	} else if requireExisting {
		return InstallResult{}, fmt.Errorf("skill %q is not installed at %s; run easy skill install %s first", selected.Name, targetPath, selected.Name)
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create target directory: %w", err)
	}
	temporary, err := os.CreateTemp(targetDir, ".SKILL.md.*.tmp")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create temporary skill file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return InstallResult{}, fmt.Errorf("set skill file permissions: %w", err)
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return InstallResult{}, fmt.Errorf("write skill file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return InstallResult{}, fmt.Errorf("sync skill file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return InstallResult{}, fmt.Errorf("close skill file: %w", err)
	}
	if err := os.Rename(temporaryName, targetPath); err != nil {
		return InstallResult{}, fmt.Errorf("replace skill file: %w", err)
	}
	return InstallResult{Path: targetPath, Changed: true}, nil
}

func InstallPath(name string, options InstallOptions) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	workingDir := options.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
	}
	homeDir := options.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
	}
	root := homeDir
	if !options.Global {
		root = findProjectRoot(workingDir)
	}
	return filepath.Join(root, ".agents", "skills", name, "SKILL.md"), nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidSkill)
	}
	if name[len(name)-1] == '-' || name[len(name)-1] == '_' {
		return fmt.Errorf("%w: invalid name %q", ErrInvalidSkill, name)
	}
	for i, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || (i > 0 && (char == '-' || char == '_')) {
			continue
		}
		return fmt.Errorf("%w: invalid name %q", ErrInvalidSkill, name)
	}
	return nil
}

func findProjectRoot(start string) string {
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
