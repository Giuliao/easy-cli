package skills

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bytedance/easy-cli/internal/skill"
)

func TestEmbeddedSkillsLoadIntoRegistry(t *testing.T) {
	registry, err := skill.Load(FS)
	if err != nil {
		t.Fatalf("skill.Load() error = %v", err)
	}
	got, ok := registry.Get("smb-work-order")
	if !ok {
		t.Fatal("embedded skill smb-work-order not found")
	}
	if got.Description == "" || got.Body == "" {
		t.Fatalf("embedded skill metadata/body is incomplete: %+v", got)
	}
	if _, ok := registry.Get("mysql-ddl-export"); !ok {
		t.Fatal("embedded skill mysql-ddl-export not found")
	}
	master, ok := registry.Get("easy-cli")
	if !ok {
		t.Fatal("embedded skill easy-cli not found")
	}
	if !strings.Contains(master.Body, "easy skill prompt <skill-name>") || !strings.Contains(master.Body, "不要因为识别到某个 skill 就执行") || !strings.Contains(master.Body, "easy mysql query") {
		t.Fatalf("easy-cli body misses usage routing rule: %q", master.Body)
	}
	if err := fstest.TestFS(FS, "easy-cli/SKILL.md", "smb-work-order/SKILL.md", "mysql-ddl-export/SKILL.md"); err != nil {
		t.Fatalf("embedded filesystem is invalid: %v", err)
	}
}
