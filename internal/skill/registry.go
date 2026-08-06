package skill

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
)

type Registry struct {
	skills map[string]Skill
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
		registry.skills[parsed.Name] = parsed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return registry, nil
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
