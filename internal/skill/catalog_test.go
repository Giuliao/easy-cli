package skill

import (
	"strings"
	"testing"
)

func TestRenderAggregateIndexesExternalMetadataWithoutBody(t *testing.T) {
	base, err := Parse("---\nname: easy-cli\ndescription: Aggregate.\n---\nBefore.\n\n" + externalSkillsStart + "\n" + externalSkillsEnd + "\n\nAfter.\n")
	if err != nil {
		t.Fatal(err)
	}
	base.Origin = OriginBuiltin
	base.SourcePath = "easy-cli/SKILL.md"
	registry := &Registry{skills: map[string]Skill{
		"easy-cli": base,
		"external": {
			Name:        "external",
			Description: "External description.",
			Source:      "---\nname: external\ndescription: External description.\n---\nSecret body.\n",
			Origin:      OriginProject,
			SourcePath:  "/tmp/external/SKILL.md",
		},
		"builtin": {
			Name:        "builtin",
			Description: "Built-in description.",
			Origin:      OriginBuiltin,
		},
	}}

	rendered, err := RenderAggregate(base, registry)
	if err != nil {
		t.Fatalf("RenderAggregate() error = %v", err)
	}
	if !strings.Contains(rendered.Source, "external") || !strings.Contains(rendered.Source, "easy skill prompt external") {
		t.Fatalf("rendered source = %q, want external catalog entry", rendered.Source)
	}
	if strings.Contains(rendered.Source, "Secret body") || strings.Contains(rendered.Source, "builtin description") {
		t.Fatalf("rendered source = %q, want metadata-only external catalog", rendered.Source)
	}
	if !strings.Contains(rendered.Source, "Before.") || !strings.Contains(rendered.Source, "After.") {
		t.Fatalf("rendered source = %q, want content outside catalog preserved", rendered.Source)
	}
}

func TestRenderAggregateRejectsMissingMarkers(t *testing.T) {
	base, err := Parse("---\nname: easy-cli\ndescription: Aggregate.\n---\nBody.\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = RenderAggregate(base, &Registry{skills: map[string]Skill{}})
	if err == nil || !strings.Contains(err.Error(), "missing external skill catalog start marker") {
		t.Fatalf("RenderAggregate() error = %v, want missing-marker error", err)
	}
}
