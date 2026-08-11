package main

import (
	"bytes"
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

func writeMainConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
