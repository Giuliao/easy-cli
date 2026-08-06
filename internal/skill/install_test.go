package skill

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallWritesCompressedSkillToProjectRoot(t *testing.T) {
	project := t.TempDir()
	nested := filepath.Join(project, "internal", "worker")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	parsed, err := Parse("---\nname: demo\ndescription: A demo skill.\n---\n\n# demo\n\n| Key | Value |\n| --- | --- |\n| API | /srv/api |\n")
	if err != nil {
		t.Fatal(err)
	}

	result, err := Install(parsed, InstallOptions{WorkingDir: nested, HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	wantPath := filepath.Join(project, ".agents", "skills", "demo", "SKILL.md")
	if result.Path != wantPath {
		t.Fatalf("Path = %q, want %q", result.Path, wantPath)
	}
	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nname: demo\ndescription: A demo skill.\n---\n- Key: API; Value: /srv/api\n"
	if string(content) != want {
		t.Fatalf("installed content = %q, want %q", content, want)
	}
}

func TestInstallRejectsNamesThatEndWithSeparator(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Install(Skill{
		Name:        "demo-",
		Description: "A demo skill.",
		Source:      "---\nname: demo-\ndescription: A demo skill.\n---\nbody\n",
	}, InstallOptions{WorkingDir: project, HomeDir: t.TempDir()})
	if !errors.Is(err, ErrInvalidSkill) {
		t.Fatal("Install() error = nil, want invalid name error")
	}
}

func TestInstallUsesWorkingDirectoryWhenGitRootIsAbsent(t *testing.T) {
	workingDir := t.TempDir()
	parsed, err := Parse("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")
	if err != nil {
		t.Fatal(err)
	}

	result, err := Install(parsed, InstallOptions{WorkingDir: workingDir, HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	want := filepath.Join(workingDir, ".agents", "skills", "demo", "SKILL.md")
	if result.Path != want {
		t.Fatalf("Path = %q, want %q", result.Path, want)
	}
}

func TestInstallPathUsesHomeDirectoryForGlobalScope(t *testing.T) {
	home := t.TempDir()
	path, err := InstallPath("demo", InstallOptions{Global: true, HomeDir: home})
	if err != nil {
		t.Fatalf("InstallPath() error = %v", err)
	}
	want := filepath.Join(home, ".agents", "skills", "demo", "SKILL.md")
	if path != want {
		t.Fatalf("InstallPath() = %q, want %q", path, want)
	}
}

func TestInstallGlobalWritesUnderHomeDirectory(t *testing.T) {
	home := t.TempDir()
	parsed, err := Parse("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")
	if err != nil {
		t.Fatal(err)
	}

	result, err := Install(parsed, InstallOptions{Global: true, WorkingDir: t.TempDir(), HomeDir: home})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	want := filepath.Join(home, ".agents", "skills", "demo", "SKILL.md")
	if result.Path != want {
		t.Fatalf("Path = %q, want %q", result.Path, want)
	}
}

func TestInstallIsIdempotentForSameContent(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install(parsed, InstallOptions{WorkingDir: project, HomeDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	result, err := Install(parsed, InstallOptions{WorkingDir: project, HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if result.Changed {
		t.Fatal("second Install() Changed = true, want false")
	}
}

func TestInstallRejectsConflictUnlessForced(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse("---\nname: demo\ndescription: A demo skill.\n---\nbody\n")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, ".agents", "skills", "demo")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(path, "SKILL.md")
	if err := os.WriteFile(target, []byte("different\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(parsed, InstallOptions{WorkingDir: project, HomeDir: t.TempDir()}); err == nil {
		t.Fatal("Install() error = nil, want conflict error")
	}
	result, err := Install(parsed, InstallOptions{WorkingDir: project, HomeDir: t.TempDir(), Force: true})
	if err != nil {
		t.Fatalf("forced Install() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("forced Install() Changed = false, want true")
	}
}
