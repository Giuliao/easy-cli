package main

import (
	"bytes"
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
