package skills

import (
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
	if err := fstest.TestFS(FS, "smb-work-order/SKILL.md"); err != nil {
		t.Fatalf("embedded filesystem is invalid: %v", err)
	}
}
