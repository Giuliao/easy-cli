package skill

import (
	"testing"
	"testing/fstest"
)

func TestLoadDiscoversAndSortsSkills(t *testing.T) {
	fsys := fstest.MapFS{
		"zeta/SKILL.md":  &fstest.MapFile{Data: []byte("---\nname: zeta\ndescription: Zeta.\n---\nz\n")},
		"alpha/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: alpha\ndescription: Alpha.\n---\na\n")},
	}

	registry, err := Load(fsys)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := registry.List()
	if len(got) != 2 {
		t.Fatalf("List() length = %d, want 2", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("List() names = %q, %q, want alpha, zeta", got[0].Name, got[1].Name)
	}
}

func TestLoadRejectsDuplicateSkillNames(t *testing.T) {
	fsys := fstest.MapFS{
		"one/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: One.\n---\n")},
		"two/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\ndescription: Two.\n---\n")},
	}

	if _, err := Load(fsys); err == nil {
		t.Fatal("Load() error = nil, want duplicate name error")
	}
}
