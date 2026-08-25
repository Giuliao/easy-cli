package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsesEmbeddedSkillRegistry(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "list"}, &stdout, &stderr, t.TempDir(), t.TempDir())

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "smb-work-order") {
		t.Fatalf("stdout = %q, want embedded skill name", stdout.String())
	}
}

func TestRunLoadsProjectConfigurationOverHome(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMainConfig(t, filepath.Join(home, ".config", "easy-cli", "config.json"), `{"smb":{"backend_repo":"/home/backend"}}`)
	writeMainConfig(t, filepath.Join(project, ".easy-cli", "config.json"), `{"smb":{"backend_repo":"/project/backend"}}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"config", "get", "smb.backend-repo"}, &stdout, &stderr, project, home)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "/project/backend\n" {
		t.Fatalf("stdout = %q, want project configuration value", stdout.String())
	}
}

func TestRunDiscoversProjectExternalSkill(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExternalSkill(t, filepath.Join(project, ".easy-cli", "skills", "project-guide", "SKILL.md"), `---
name: project-guide
description: Project guide.
---
Project-only instructions.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "prompt", "project-guide"}, &stdout, &stderr, project, t.TempDir())

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Project-only instructions.") {
		t.Fatalf("stdout = %q, want project skill body", stdout.String())
	}
}

func TestRunProjectExternalSkillOverridesHomeSkill(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExternalSkill(t, filepath.Join(home, ".config", "easy-cli", "skills", "shared", "SKILL.md"), `---
name: shared
description: Home guide.
---
Home instructions.
`)
	writeExternalSkill(t, filepath.Join(project, ".easy-cli", "skills", "shared", "SKILL.md"), `---
name: shared
description: Project guide.
---
Project instructions.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "prompt", "shared"}, &stdout, &stderr, project, home)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Project instructions.") || strings.Contains(stdout.String(), "Home instructions.") {
		t.Fatalf("stdout = %q, want project skill to override home skill", stdout.String())
	}
}

func TestRunRejectsExternalEasyCLIOverride(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".easy-cli", "skills", "easy-cli", "SKILL.md")
	writeExternalSkill(t, path, `---
name: easy-cli
description: Replacement.
---
Replacement instructions.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "list"}, &stdout, &stderr, project, t.TempDir())

	if code != 1 {
		t.Fatalf("run() code = %d, want 1; stdout = %q; stderr = %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "easy-cli") || !strings.Contains(stderr.String(), path) {
		t.Fatalf("stderr = %q, want reserved-name error with source path", stderr.String())
	}
}

func TestRunPromptJSONIncludesExternalSkillOrigin(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".easy-cli", "skills", "json-guide", "SKILL.md")
	writeExternalSkill(t, path, `---
name: json-guide
description: JSON guide.
---
JSON instructions.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "prompt", "json-guide", "--format", "json"}, &stdout, &stderr, project, t.TempDir())

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	var payload struct {
		Origin     string `json:"origin"`
		SourcePath string `json:"source_path"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("JSON output is invalid: %v", err)
	}
	if payload.Origin != "project" || payload.SourcePath != path {
		t.Fatalf("payload origin/path = %q/%q, want project/%q", payload.Origin, payload.SourcePath, path)
	}
}

func TestRunInstallEasyCLIIncludesExternalCatalogWithoutBody(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExternalSkill(t, filepath.Join(home, ".config", "easy-cli", "skills", "catalog-guide", "SKILL.md"), `---
name: catalog-guide
description: Catalog guide.
---
Secret external body that must stay out of the catalog.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "install", "easy-cli"}, &stdout, &stderr, project, home)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	content, err := os.ReadFile(filepath.Join(project, ".agents", "skills", "easy-cli", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "catalog-guide") || !strings.Contains(text, "easy skill prompt catalog-guide") {
		t.Fatalf("installed easy-cli = %q, want external catalog entry", text)
	}
	if strings.Contains(text, "Secret external body") {
		t.Fatalf("installed easy-cli contains external skill body: %q", text)
	}
}

func TestRunPromptEasyCLIIncludesExternalCatalogWithoutBody(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExternalSkill(t, filepath.Join(project, ".easy-cli", "skills", "prompt-guide", "SKILL.md"), `---
name: prompt-guide
description: Prompt guide.
---
Prompt-only external body.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "prompt", "easy-cli"}, &stdout, &stderr, project, t.TempDir())

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "prompt-guide") || !strings.Contains(stdout.String(), "easy skill prompt prompt-guide") {
		t.Fatalf("prompt = %q, want external catalog entry", stdout.String())
	}
	if strings.Contains(stdout.String(), "Prompt-only external body.") {
		t.Fatalf("prompt contains external skill body: %q", stdout.String())
	}
}

func TestRunUpdateEasyCLIRefreshesExternalCatalog(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	var installOut, installErr bytes.Buffer
	if code := run([]string{"skill", "install", "easy-cli"}, &installOut, &installErr, project, home); code != 0 {
		t.Fatalf("install run() code = %d; stderr = %q", code, installErr.String())
	}
	writeExternalSkill(t, filepath.Join(project, ".easy-cli", "skills", "updated-guide", "SKILL.md"), `---
name: updated-guide
description: Updated guide.
---
Updated instructions.
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "update", "easy-cli"}, &stdout, &stderr, project, home)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	content, err := os.ReadFile(filepath.Join(project, ".agents", "skills", "easy-cli", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "updated-guide") {
		t.Fatalf("updated easy-cli = %q, want refreshed external catalog", content)
	}
}

func writeMainConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExternalSkill(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
