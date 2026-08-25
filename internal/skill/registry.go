package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/bytedance/easy-cli/internal/projectroot"
)

type Registry struct {
	skills map[string]Skill
}

type DiscoveryOptions struct {
	WorkingDir string
	HomeDir    string
}

func Load(fsys fs.FS) (*Registry, error) {
	registry := &Registry{skills: make(map[string]Skill)}
	err := fs.WalkDir(fsys, ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Base(filePath) != "SKILL.md" {
			return nil
		}
		content, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return err
		}
		parsed, err := Parse(string(content))
		if err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
		if _, exists := registry.skills[parsed.Name]; exists {
			return fmt.Errorf("%w: duplicate name %q", ErrInvalidSkill, parsed.Name)
		}
		parsed.Origin = OriginBuiltin
		parsed.SourcePath = filePath
		registry.skills[parsed.Name] = parsed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return registry, nil
}

func LoadAll(fsys fs.FS, options DiscoveryOptions) (*Registry, error) {
	registry, err := Load(fsys)
	if err != nil {
		return nil, err
	}

	workingDir := options.WorkingDir
	if workingDir == "" {
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	homeDir := options.HomeDir
	if homeDir == "" {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
	}

	sources := []struct {
		root   string
		origin Origin
	}{
		{root: filepath.Join(homeDir, ".config", "easy-cli", "skills"), origin: OriginHome},
		{root: filepath.Join(projectroot.Find(workingDir), ".easy-cli", "skills"), origin: OriginProject},
	}
	for _, source := range sources {
		if err := registry.loadExternal(source.root, source.origin); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) loadExternal(root string, origin Origin) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s skill directory: %w", origin, err)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve %s skill directory: %w", origin, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(absoluteRoot, entry.Name())
		filePath := filepath.Join(directory, "SKILL.md")
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read external skill %s: %w", filePath, err)
		}
		parsed, err := Parse(string(content))
		if err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
		if parsed.Name != entry.Name() {
			return fmt.Errorf("%s: %w: directory name %q does not match skill name %q", filePath, ErrInvalidSkill, entry.Name(), parsed.Name)
		}
		if parsed.Name == "easy-cli" {
			return fmt.Errorf("%s: %w: reserved skill name %q", filePath, ErrInvalidSkill, parsed.Name)
		}
		if err := validateName(parsed.Name); err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
		parsed.Origin = origin
		parsed.SourcePath = filePath
		r.skills[parsed.Name] = parsed
	}
	return nil
}

func (r *Registry) Get(name string) (Skill, bool) {
	skill, ok := r.skills[name]
	return skill, ok
}

func (r *Registry) List() []Skill {
	result := make([]Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		result = append(result, skill)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (r *Registry) External() []Skill {
	result := make([]Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		if skill.Origin == OriginBuiltin {
			continue
		}
		result = append(result, skill)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
