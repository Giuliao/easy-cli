package config

import (
	"errors"
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
  }
}`)
	writeConfigFile(t, filepath.Join(project, ".easy-cli", "config.json"), `{
  "mysql": {
    "host": "project.db.internal",
    "database": "project-db"
  }
}`)

	got, err := Load(LoadOptions{WorkingDir: workingDir, HomeDir: home})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.MySQL.Host != "project.db.internal" || got.MySQL.Port != 3306 || got.MySQL.User != "home-user" || got.MySQL.Password != "home-password" || got.MySQL.Database != "project-db" {
		t.Fatalf("MySQL = %+v, want field-level merge", got.MySQL)
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
	}

	got, err := config.Get("mysql.host")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "db.internal" {
		t.Fatalf("Get() = %q, want host", got)
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
	_, err := (Config{}).Get("mysql.host")
	if err == nil {
		t.Fatal("Get() error = nil, want unset-value error")
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Fatalf("Get() error = %q, want not-set error", err)
	}
}

func TestInitHomeCreatesPrivateValidTemplate(t *testing.T) {
	home := t.TempDir()
	result, err := InitHome(InitOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("InitHome() error = %v", err)
	}
	wantPath := filepath.Join(home, ".config", "easy-cli", "config.json")
	if result.Path != wantPath || !result.Changed {
		t.Fatalf("InitHome() = %+v, want created path %q", result, wantPath)
	}
	fileInfo, err := os.Stat(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("config file permissions = %o, want 600", fileInfo.Mode().Perm())
	}
	directoryInfo, err := os.Stat(filepath.Dir(wantPath))
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config directory permissions = %o, want 700", directoryInfo.Mode().Perm())
	}
	loaded, err := Load(LoadOptions{WorkingDir: t.TempDir(), HomeDir: home})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != (Config{MySQL: MySQL{Port: 3306}}) {
		t.Fatalf("initialized config = %+v, want empty template with default port", loaded)
	}
}

func TestInitHomePreservesExistingConfigurationWithoutForce(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "easy-cli", "config.json")
	contents := `{"mysql":{"host":"keep.db.internal"}}`
	writeConfigFile(t, path, contents)

	_, err := InitHome(InitOptions{HomeDir: home})
	if !errors.Is(err, ErrConfigExists) {
		t.Fatalf("InitHome() error = %v, want ErrConfigExists", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != contents {
		t.Fatalf("existing config = %q, want preserved %q", after, contents)
	}
}

func TestInitHomeForceReplacesExistingConfiguration(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "easy-cli", "config.json")
	writeConfigFile(t, path, `{"mysql":{"host":"old.db.internal"}}`)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := InitHome(InitOptions{HomeDir: home, Force: true})
	if err != nil {
		t.Fatalf("InitHome() error = %v", err)
	}
	if result.Path != path || !result.Changed {
		t.Fatalf("InitHome() = %+v, want forced replacement", result)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "old.db.internal") {
		t.Fatalf("config content = %q, want initialized template", contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config file permissions = %o, want 600", info.Mode().Perm())
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
