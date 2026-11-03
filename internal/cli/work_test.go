package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Giuliao/easy-cli/internal/project"
	"github.com/Giuliao/easy-cli/internal/skill"
)

func cliWorkGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func cliWorkRun(t *testing.T, home, dir string, args ...string) (int, string, string) {
	t.Helper()
	registry, err := skill.Load(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run(args, registry, Options{HomeDir: home, WorkingDir: dir, Out: &out, ErrOut: &errOut})
	return code, out.String(), errOut.String()
}

func cliWorkOK(t *testing.T, home, dir string, args ...string) string {
	t.Helper()
	code, out, errOut := cliWorkRun(t, home, dir, args...)
	if code != 0 {
		t.Fatalf("%v: code %d stderr %s", args, code, errOut)
	}
	return out
}

func cliWorkState(t *testing.T, out string) project.WorkState {
	t.Helper()
	var state project.WorkState
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		t.Fatalf("state JSON %s: %v", out, err)
	}
	return state
}

func cliWorkSetup(t *testing.T) (home, dir string) {
	t.Helper()
	home, dir = t.TempDir(), t.TempDir()
	cliWorkGit(t, dir, "init", "-b", "main")
	cliWorkGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "base")
	return home, dir
}

func TestWorkCLIRestartResumeAndCheckpoint(t *testing.T) {
	home, dir := cliWorkSetup(t)
	cliWorkOK(t, home, dir, "project", "create", "feature")
	cliWorkOK(t, home, dir, "project", "use", "feature")
	first := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--agent", "codex", "--json"))
	second := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--agent", "claude", "--json"))
	if first.Task.ID != second.Task.ID || second.Created || second.Task.SessionID != "" {
		t.Fatalf("new invocation created task: %+v", second)
	}
	cliWorkGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "change")
	cp := cliWorkState(t, cliWorkOK(t, home, dir, "work", "checkpoint", "--json"))
	if cp.Checkpoint == nil || !strings.HasPrefix(cp.Task.CommitRange, first.Git.Head+"..") || cp.Git.Head == first.Git.Head {
		t.Fatalf("checkpoint = %+v", cp)
	}
	var history []project.Checkpoint
	if err := json.Unmarshal([]byte(cliWorkOK(t, home, dir, "work", "history", "--json")), &history); err != nil || len(history) != 1 {
		t.Fatalf("history %+v %v", history, err)
	}
	if out := cliWorkOK(t, home, dir, "work", "status"); !strings.Contains(out, "Project: feature") || !strings.Contains(out, first.Task.ID) {
		t.Fatal(out)
	}
	var shown struct {
		Workspaces []project.Workspace `json:"workspaces"`
		Tasks      []project.Task      `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(cliWorkOK(t, home, t.TempDir(), "project", "show", "feature", "--json")), &shown); err != nil || len(shown.Workspaces) != 1 || len(shown.Tasks) != 1 {
		t.Fatalf("project show %+v %v", shown, err)
	}
	if out := cliWorkOK(t, home, dir, "project", "show", "feature"); !strings.Contains(out, "Workspaces:") || !strings.Contains(out, "ACTIVE") {
		t.Fatal(out)
	}
}

func TestWorkCLIExplicitSelectionConflictAndLifecycle(t *testing.T) {
	home, dir := cliWorkSetup(t)
	cliWorkOK(t, home, dir, "project", "create", "feature")
	cliWorkOK(t, home, dir, "project", "create", "other")
	first := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--project", "feature", "--json"))
	for _, args := range [][]string{
		{"work", "ensure", "--project", "other"},
		{"project", "use", "other"},
		{"project", "close", "feature"},
		{"work", "ensure", "--task", first.Task.ID, "--new-task"},
	} {
		if code, _, _ := cliWorkRun(t, home, dir, args...); code == 0 {
			t.Fatalf("unsafe command accepted: %v", args)
		}
	}
	cliWorkOK(t, home, dir, "project", "close", "feature", "--confirm")
	if code, _, _ := cliWorkRun(t, home, dir, "work", "ensure"); code == 0 {
		t.Fatal("ended resume accepted")
	}
	if code, _, _ := cliWorkRun(t, home, dir, "project", "reopen", "feature"); code == 0 {
		t.Fatal("unconfirmed reopen accepted")
	}
	cliWorkOK(t, home, dir, "project", "reopen", "feature", "--confirm")
	second := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--new-task", "--json"))
	if second.Task.ID == first.Task.ID || second.Workspace.ID != first.Workspace.ID {
		t.Fatalf("new segment = %+v", second)
	}
	selected := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--task", first.Task.ID, "--json"))
	if selected.Task.ID != first.Task.ID {
		t.Fatalf("selection = %+v", selected)
	}
	cliWorkOK(t, home, dir, "work", "checkpoint")
	cliWorkOK(t, home, dir, "project", "use", "other", "--replace")
	var archivedHistory []project.Checkpoint
	if err := json.Unmarshal([]byte(cliWorkOK(t, home, t.TempDir(), "work", "history", "--workspace", first.Workspace.ID, "--json")), &archivedHistory); err != nil || len(archivedHistory) != 1 {
		t.Fatalf("archived history %+v %v", archivedHistory, err)
	}
	if code, _, _ := cliWorkRun(t, home, dir, "work", "ensure"); code == 0 {
		t.Fatal("replacement inherited task without selection")
	}
	third := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--new-task", "--json"))
	if third.Project.Name != "other" || third.Workspace.ID == first.Workspace.ID || third.Task.ID == first.Task.ID {
		t.Fatalf("replacement = %+v", third)
	}
}

func TestTaskCreateSessionOptionalAndTargetDirectoryContext(t *testing.T) {
	home, dir := cliWorkSetup(t)
	cliWorkOK(t, home, dir, "project", "create", "feature")
	cliWorkOK(t, home, dir, "project", "use", "feature")
	cliWorkOK(t, home, t.TempDir(), "task", "create", "--agent", "codex", "--branch", "main", "--dir", dir)
	var tasks []project.Task
	if err := json.Unmarshal([]byte(cliWorkOK(t, home, dir, "task", "list", "--project", "feature", "--json")), &tasks); err != nil || len(tasks) != 1 || tasks[0].SessionID != "" {
		t.Fatalf("tasks %+v %v", tasks, err)
	}
	before := len(tasks)
	if code, _, _ := cliWorkRun(t, home, dir, "task", "create", "--agent", "codex", "--branch", "main", "--dir", dir, "--project", "missing"); code == 0 {
		t.Fatal("missing project accepted")
	}
	if err := json.Unmarshal([]byte(cliWorkOK(t, home, dir, "task", "list", "--json")), &tasks); err != nil || len(tasks) != before {
		t.Fatalf("failed create orphaned task %+v %v", tasks, err)
	}
}

func TestWorkCLIDirRelativeAndNoBinding(t *testing.T) {
	home, dir := cliWorkSetup(t)
	if code, _, errOut := cliWorkRun(t, home, dir, "work", "ensure"); code == 0 || !strings.Contains(errOut, "not bound") {
		t.Fatalf("unbound: %d %s", code, errOut)
	}
	cliWorkOK(t, home, dir, "project", "create", "feature")
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	state := cliWorkState(t, cliWorkOK(t, home, dir, "work", "ensure", "--project", "feature", "--dir", "nested", "--json"))
	if !state.Created {
		t.Fatal("missing task")
	}
	cliWorkOK(t, home, nested, "work", "status", "--dir", "..")
}
