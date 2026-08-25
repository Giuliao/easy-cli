package skill

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidSkill = errors.New("invalid skill")

type Origin string

const (
	OriginBuiltin Origin = "builtin"
	OriginHome    Origin = "home"
	OriginProject Origin = "project"
)

type Skill struct {
	Name        string
	Description string
	Source      string
	Body        string
	Origin      Origin
	SourcePath  string
}

func Parse(source string) (Skill, error) {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Skill{}, fmt.Errorf("%w: missing front matter", ErrInvalidSkill)
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return Skill{}, fmt.Errorf("%w: unclosed front matter", ErrInvalidSkill)
	}

	metadata := make(map[string]string)
	for _, line := range lines[1:end] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metadata[key] = unquote(strings.TrimSpace(value))
	}

	name := metadata["name"]
	if name == "" {
		return Skill{}, fmt.Errorf("%w: missing name", ErrInvalidSkill)
	}
	description := metadata["description"]
	if description == "" {
		return Skill{}, fmt.Errorf("%w: missing description", ErrInvalidSkill)
	}

	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if body != "" {
		body += "\n"
	}
	return Skill{
		Name:        name,
		Description: description,
		Source:      source,
		Body:        body,
	}, nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
