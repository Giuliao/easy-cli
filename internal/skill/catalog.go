package skill

import (
	"fmt"
	"strings"
)

const (
	ExternalSkillsStart = "<!-- EASY_EXTERNAL_SKILLS_START -->"
	ExternalSkillsEnd   = "<!-- EASY_EXTERNAL_SKILLS_END -->"
)

const (
	externalSkillsStart = ExternalSkillsStart
	externalSkillsEnd   = ExternalSkillsEnd
)

func RenderAggregate(selected Skill, registry *Registry) (Skill, error) {
	if selected.Name != "easy-cli" {
		return selected, nil
	}
	start := strings.Index(selected.Source, externalSkillsStart)
	if start < 0 {
		return Skill{}, fmt.Errorf("skill %q is missing external skill catalog start marker", selected.Name)
	}
	end := strings.Index(selected.Source[start+len(externalSkillsStart):], externalSkillsEnd)
	if end < 0 {
		return Skill{}, fmt.Errorf("skill %q is missing external skill catalog end marker", selected.Name)
	}
	end += start + len(externalSkillsStart)

	content := strings.Builder{}
	content.WriteString(externalSkillsStart)
	content.WriteString("\n\n## External skills\n\n")
	external := registry.External()
	if len(external) == 0 {
		content.WriteString("No external skills are configured.\n")
	} else {
		for _, selected := range external {
			fmt.Fprintf(&content, "- `%s` — %s (%s)\n  Load with: `easy skill prompt %s`\n", selected.Name, singleLine(selected.Description), selected.Origin, selected.Name)
		}
	}
	content.WriteString(externalSkillsEnd)

	rendered := selected.Source[:start] + content.String() + selected.Source[end+len(externalSkillsEnd):]
	parsed, err := Parse(rendered)
	if err != nil {
		return Skill{}, fmt.Errorf("render skill %q: %w", selected.Name, err)
	}
	parsed.Origin = selected.Origin
	parsed.SourcePath = selected.SourcePath
	return parsed, nil
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
