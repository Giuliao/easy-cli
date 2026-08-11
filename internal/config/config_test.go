package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesProjectConfigOverHomeByField(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	workingDir := filepath.Join(project, "nested", "service")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfigFile(t, filepath.Join(home, ".config", "easy-cli", "config.json"), `{
  "mysql": {
    "host": "home.db.internal",
    "port": 3306,
    "user": "home-user",
    "password": "home-password",
    "database": "home-db"
  },
  "smb": {
    "backend_repo": "/home/backend",
    "frontend_repo": "/home/frontend",
    "idl_repo": "/home/idl"
  }
}`)
	writeConfigFile(t, filepath.Join(project, ".easy-cli", "config.json"), `{
  "mysql": {
    "host": "project.db.internal",
    "database": "project-db"
  },
  "smb": {
    "backend_repo": "/project/backend"
  }
}`)

	got, err := Load(LoadOptions{WorkingDir: workingDir, HomeDir: home})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.MySQL.Host != "project.db.internal" || got.MySQL.Port != 3306 || got.MySQL.User != "home-user" || got.MySQL.Password != "home-password" || got.MySQL.Database != "project-db" {
		t.Fatalf("MySQL = %+v, want field-level merge", got.MySQL)
	}
	if got.SMB.BackendRepo != "/project/backend" || got.SMB.FrontendRepo != "/home/frontend" || got.SMB.IDLRepo != "/home/idl" {
		t.Fatalf("SMB = %+v, want field-level merge", got.SMB)
	}
}

func TestLoadAllowsAbsentConfigFiles(t *testing.T) {
	got, err := Load(LoadOptions{WorkingDir: t.TempDir(), HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != (Config{}) {
		t.Fatalf("Load() = %+v, want empty config", got)
	}
}

func TestLoadRejectsInvalidOrUnknownConfigWithoutLeakingPassword(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{name: "invalid JSON", contents: `{"mysql":`},
		{name: "unknown field", contents: `{"mysql":{"password":"secret-value","unexpected":"value"}}`},
		{name: "wrong field type", contents: `{"mysql":{"port":"3306"}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".config", "easy-cli", "config.json")
			writeConfigFile(t, path, test.contents)

			_, err := Load(LoadOptions{WorkingDir: t.TempDir(), HomeDir: home})
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("Load() error = %q, want config path", err)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("Load() leaked password: %q", err)
			}
		})
	}
}

func TestGetReturnsOnlyAllowedNonSensitiveValues(t *testing.T) {
	config := Config{
		MySQL: MySQL{Host: "db.internal", Port: 3307, User: "app", Password: "secret-value", Database: "orders"},
		SMB:   SMB{BackendRepo: "/code/backend", FrontendRepo: "/code/frontend", IDLRepo: "/code/idl"},
	}

	got, err := config.Get("smb.backend-repo")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "/code/backend" {
		t.Fatalf("Get() = %q, want backend repo", got)
	}

	_, err = config.Get("mysql.password")
	if err == nil {
		t.Fatal("Get(mysql.password) error = nil, want rejected key")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("Get(mysql.password) leaked password: %q", err)
	}
}

func TestGetRejectsUnsetValue(t *testing.T) {
	_, err := (Config{}).Get("smb.idl-repo")
	if err == nil {
		t.Fatal("Get() error = nil, want unset-value error")
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Fatalf("Get() error = %q, want not-set error", err)
	}
}

func writeConfigFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
