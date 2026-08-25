package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadAllMergesExternalSkillSourcesByPrecedence(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, filepath.Join(home, ".config", "easy-cli", "skills", "shared", "SKILL.md"), "shared", "Home shared.")
	writeSkillFile(t, filepath.Join(home, ".config", "easy-cli", "skills", "home-only", "SKILL.md"), "home-only", "Home only.")
	projectShared := filepath.Join(project, ".easy-cli", "skills", "shared", "SKILL.md")
	writeSkillFile(t, projectShared, "shared", "Project shared.")

	registry, err := LoadAll(fstest.MapFS{
		"builtin/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: builtin\ndescription: Built-in.\n---\nBuilt-in.\n")},
	}, DiscoveryOptions{WorkingDir: project, HomeDir: home})
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	builtin, ok := registry.Get("builtin")
	if !ok || builtin.Origin != OriginBuiltin || builtin.SourcePath != "builtin/SKILL.md" {
		t.Fatalf("builtin skill = %+v, want builtin origin/path", builtin)
	}
	homeOnly, ok := registry.Get("home-only")
	if !ok || homeOnly.Origin != OriginHome {
		t.Fatalf("home-only skill = %+v, want home origin", homeOnly)
	}
	shared, ok := registry.Get("shared")
	if !ok || shared.Origin != OriginProject || shared.SourcePath != projectShared || !strings.Contains(shared.Body, "Project shared.") {
		t.Fatalf("shared skill = %+v, want project override", shared)
	}
}

func TestLoadAllRejectsExternalDirectoryNameMismatch(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "easy-cli", "skills", "wrong-directory", "SKILL.md")
	writeSkillFile(t, path, "declared-name", "Body.")

	_, err := LoadAll(fstest.MapFS{}, DiscoveryOptions{HomeDir: home, WorkingDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "does not match") || !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadAll() error = %v, want path/name mismatch", err)
	}
}

func TestLoadAllRejectsExternalEasyCLIName(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "easy-cli", "skills", "easy-cli", "SKILL.md")
	writeSkillFile(t, path, "easy-cli", "Replacement.")

	_, err := LoadAll(fstest.MapFS{}, DiscoveryOptions{HomeDir: home, WorkingDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "reserved skill name") || !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadAll() error = %v, want reserved-name error", err)
	}
}

func writeSkillFile(t *testing.T, path, name, body string) {
	t.Helper()
	contents := "---\nname: " + name + "\ndescription: Test skill.\n---\n" + body + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
