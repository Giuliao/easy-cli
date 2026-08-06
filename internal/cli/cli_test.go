package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/bytedance/easy-cli/internal/skill"
)

func TestRunPromptWritesOnlyCompressedPromptToStdout(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\n\n# demo\n\n| Key | Value |\n| --- | --- |\n| API | /srv/api |\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "prompt", "demo"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "---\nname: demo\ndescription: A demo skill.\n---\n- Key: API; Value: /srv/api\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunPromptRawReturnsSource(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\n\n# demo\n\nKeep this source.\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "prompt", "demo", "--raw"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "---\nname: demo\ndescription: A demo skill.\n---\n\n# demo\n\nKeep this source.\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunPromptJSONIncludesRawAndCompressedContent(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\n\n# demo\n\nKeep this source.\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "prompt", "demo", "--format", "json"}, registry, Options{Out: &stdout, ErrOut: &stderr})
	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	var payload struct {
		Name   string `json:"name"`
		Raw    string `json:"raw"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("JSON output is invalid: %v", err)
	}
	if payload.Name != "demo" || payload.Raw == "" || payload.Prompt == "" {
		t.Fatalf("JSON payload = %+v, want name/raw/prompt", payload)
	}
	if payload.Raw == payload.Prompt {
		t.Fatal("raw and compressed prompt unexpectedly match")
	}
}

func TestRunSkillListPrintsSortedNameAndDescription(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"zeta/SKILL.md":  &fstest.MapFile{Data: []byte("---\nname: zeta\ndescription: Zeta.\n---\nz\n")},
		"alpha/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: alpha\ndescription: Alpha.\n---\na\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "list"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "alpha\tAlpha.\tnot-installed\nzeta\tZeta.\tnot-installed\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunSkillListShowsInstalledStatus(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	selected, _ := registry.Get("demo")
	if _, err := skill.Install(selected, skill.InstallOptions{WorkingDir: project, HomeDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "list"}, registry, Options{
		WorkingDir: project,
		HomeDir:    t.TempDir(),
		Out:        &stdout,
		ErrOut:     &stderr,
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "demo\tA demo skill.\tinstalled\n" {
		t.Fatalf("stdout = %q, want installed status", stdout.String())
	}
}

func TestRunSkillShowPrintsMetadata(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "show", "demo"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "Name: demo\nDescription: A demo skill.\nStatus: not-installed\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunSkillShowIncludesInstallationStatus(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	selected, _ := registry.Get("demo")
	if _, err := skill.Install(selected, skill.InstallOptions{WorkingDir: project, HomeDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "show", "demo"}, registry, Options{
		WorkingDir: project,
		HomeDir:    t.TempDir(),
		Out:        &stdout,
		ErrOut:     &stderr,
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "Name: demo\nDescription: A demo skill.\nStatus: installed\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunSkillInstallWritesProjectSkill(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(project, ".agents", "skills", "demo", "SKILL.md")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "install", "demo"}, registry, Options{
		WorkingDir: project,
		HomeDir:    t.TempDir(),
		Out:        &stdout,
		ErrOut:     &stderr,
	})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("installed skill not found at %s: %v", target, err)
	}
	want := "Installed demo to " + target + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunSkillNameActsAsPromptShortcut(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"demo"}, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if stdout.String() != "---\nname: demo\ndescription: A demo skill.\n---\nbody\n" {
		t.Fatalf("stdout = %q, want compressed prompt", stdout.String())
	}
}

func TestRunWithoutArgumentsPrintsHelp(t *testing.T) {
	registry, err := skill.Load(fstest.MapFS{
		"demo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")},
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, registry, Options{Out: &stdout, ErrOut: &stderr})

	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	want := "Usage: easy <command>\n\nCommands:\n  skill list\n  skill show <name>\n  skill prompt <name>\n  skill install <name>\n  mysql ddl\n\nSkills:\n  demo\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}
