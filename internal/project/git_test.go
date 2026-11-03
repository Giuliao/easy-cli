package project

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type projectGitTestRepo struct {
	dir string
	env []string
}

// These fixtures use real Git, but never the developer's Git config, templates,
// hooks, index, or object database. Their environment is independent of the
// implementation's environment cleaning so that isolation tests are meaningful.
func newProjectGitTestRepo(t *testing.T, commits bool) projectGitTestRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("Git is required for integration tests: %v", err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "main repo ; $literal ")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "isolated-home")
	hooks := filepath.Join(root, "empty-hooks")
	for _, path := range []string{home, hooks} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") || key == "HOME" || key == "XDG_CONFIG_HOME" || key == "LC_ALL" {
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0="+hooks,
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
	r := projectGitTestRepo{dir: dir, env: env}
	r.git(t, "init", "-b", "main", "--template=")
	r.git(t, "config", "user.name", "Git Integration Test")
	r.git(t, "config", "user.email", "git-test@example.invalid")
	r.git(t, "config", "commit.gpgSign", "false")
	r.git(t, "config", "tag.gpgSign", "false")
	r.git(t, "config", "core.autocrlf", "false")
	if commits {
		r.commit(t, "tracked.txt", "initial\n")
	}
	return r
}

func (r projectGitTestRepo) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"--no-replace-objects", "-C", r.dir}, args...)...)
	cmd.Env = r.env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSuffix(string(output), "\n")
}

func (r projectGitTestRepo) write(t *testing.T, path, contents string) {
	t.Helper()
	path = filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r projectGitTestRepo) commit(t *testing.T, path, contents string) string {
	t.Helper()
	r.write(t, path, contents)
	r.git(t, "add", "--", path)
	r.git(t, "commit", "-m", "update "+path)
	return r.git(t, "rev-parse", "HEAD")
}

func projectGitTestCanonical(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func assertProjectGitError(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("error = %v, want actionable error containing %q", err, contains)
	}
}

func TestReadGitStateCanonicalWorktree(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	// A same-named tag must not change branch identity to "heads/main".
	r.git(t, "tag", "main")
	subdir := filepath.Join(r.dir, "nested", "deeper")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "worktree-link")
	if err := os.Symlink(r.dir, link); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, subdir)
	if err != nil {
		t.Fatal(err)
	}
	want := GitState{
		Directory:    projectGitTestCanonical(t, r.dir),
		GitCommonDir: projectGitTestCanonical(t, filepath.Join(r.dir, ".git")),
		Repository:   filepath.Base(r.dir),
		Branch:       "main",
		Head:         r.git(t, "rev-parse", "HEAD"),
	}
	for _, dir := range []string{r.dir, subdir, link, filepath.Join(link, "nested", "deeper"), relative} {
		t.Run(dir, func(t *testing.T) {
			got, err := ReadGitState(dir)
			if err != nil {
				t.Fatalf("ReadGitState: %v", err)
			}
			if got != want {
				t.Errorf("state = %+v, want %+v", got, want)
			}
		})
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 6 {
		t.Errorf("GitState JSON has %d fields, want 6", len(fields))
	}
	for _, key := range []string{"directory", "git_common_dir", "repository", "branch", "head", "dirty"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("GitState JSON missing %q", key)
		}
	}
}

func TestReadGitStateLinkedWorktrees(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	mainState, err := ReadGitState(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"feature/one", "feature/two"} {
		linked := filepath.Join(t.TempDir(), "different-name")
		r.git(t, "worktree", "add", "-b", branch, "--", linked)
		linkedRepo := projectGitTestRepo{dir: linked, env: r.env}
		head := linkedRepo.commit(t, "linked.txt", branch+"\n")
		linkedRepo.write(t, ".easy-cli/context", "project-name\n")
		subdir := filepath.Join(linked, "subdir")
		if err := os.Mkdir(subdir, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "linked-symlink")
		if err := os.Symlink(subdir, link); err != nil {
			t.Fatal(err)
		}
		state, err := ReadGitState(link)
		if err != nil {
			t.Fatal(err)
		}
		want := GitState{
			Directory:    projectGitTestCanonical(t, linked),
			GitCommonDir: mainState.GitCommonDir,
			Repository:   mainState.Repository,
			Branch:       branch,
			Head:         head,
		}
		if state != want {
			t.Errorf("linked state = %+v, want %+v", state, want)
		}
	}
	state, err := ReadGitState(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if state != mainState {
		t.Errorf("main worktree changed: %+v, want %+v", state, mainState)
	}
}

func TestReadGitStateCanonicalCommonDir(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	gitDir := filepath.Join(r.dir, ".git")
	metadata := filepath.Join(filepath.Dir(r.dir), "actual-git-metadata")
	if err := os.Rename(gitDir, metadata); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(metadata, gitDir); err != nil {
		t.Fatal(err)
	}
	state, err := ReadGitState(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := projectGitTestCanonical(t, metadata); state.GitCommonDir != want {
		t.Errorf("common directory = %q, want %q", state.GitCommonDir, want)
	}
	if state.Repository != filepath.Base(r.dir) {
		t.Errorf("repository = %q, want main worktree name %q", state.Repository, filepath.Base(r.dir))
	}
	linked := filepath.Join(t.TempDir(), "linked")
	r.git(t, "worktree", "add", "-b", "linked", "--", linked)
	_, err = ReadGitState(linked)
	// Git reports relocated metadata as the main worktree and retains no
	// backlink to its real directory. Do not invent a repository identity.
	assertProjectGitError(t, err, "cannot identify the main worktree")
	assertProjectGitError(t, err, "select the main worktree")
}

func TestReadGitStateDirty(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, projectGitTestRepo)
		dirty  bool
	}{
		{"clean", func(t *testing.T, r projectGitTestRepo) {}, false},
		{"unstaged", func(t *testing.T, r projectGitTestRepo) { r.write(t, "tracked.txt", "changed\n") }, true},
		{"staged", func(t *testing.T, r projectGitTestRepo) {
			r.write(t, "tracked.txt", "changed\n")
			r.git(t, "add", "tracked.txt")
		}, true},
		{"untracked", func(t *testing.T, r projectGitTestRepo) { r.write(t, "new\nfile.txt", "new\n") }, true},
		{"ignored", func(t *testing.T, r projectGitTestRepo) {
			r.commit(t, ".gitignore", "ignored.txt\n")
			r.write(t, "ignored.txt", "ignored\n")
		}, false},
		{"context-untracked", func(t *testing.T, r projectGitTestRepo) { r.write(t, ".easy-cli/context", "project\n") }, false},
		{"context-staged", func(t *testing.T, r projectGitTestRepo) {
			r.write(t, ".easy-cli/context", "project\n")
			r.git(t, "add", ".easy-cli/context")
		}, false},
		{"context-unstaged", func(t *testing.T, r projectGitTestRepo) {
			r.commit(t, ".easy-cli/context", "project\n")
			r.write(t, ".easy-cli/context", "other-project\n")
		}, false},
		{"context-deleted", func(t *testing.T, r projectGitTestRepo) {
			r.commit(t, ".easy-cli/context", "project\n")
			r.git(t, "rm", ".easy-cli/context")
		}, false},
		{"context-and-work", func(t *testing.T, r projectGitTestRepo) {
			r.write(t, ".easy-cli/context", "project\n")
			r.write(t, "tracked.txt", "changed\n")
		}, true},
		{"other-bookkeeping-directory-file", func(t *testing.T, r projectGitTestRepo) {
			r.write(t, ".easy-cli/context", "project\n")
			r.write(t, ".easy-cli/other", "real change\n")
		}, true},
		{"similarly-named-file", func(t *testing.T, r projectGitTestRepo) { r.write(t, ".easy-cli/context.bak", "new\n") }, true},
		{"nested-context", func(t *testing.T, r projectGitTestRepo) { r.write(t, "nested/.easy-cli/context", "new\n") }, true},
		{"rename-into-context", func(t *testing.T, r projectGitTestRepo) {
			if err := os.Mkdir(filepath.Join(r.dir, ".easy-cli"), 0o755); err != nil {
				t.Fatal(err)
			}
			r.git(t, "mv", "tracked.txt", ".easy-cli/context")
		}, true},
		{"rename-out-of-context", func(t *testing.T, r projectGitTestRepo) {
			r.commit(t, ".easy-cli/context", "project\n")
			r.git(t, "mv", ".easy-cli/context", "real-work.txt")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newProjectGitTestRepo(t, true)
			tt.mutate(t, r)
			beforeHead := r.git(t, "rev-parse", "HEAD")
			beforeStatus := r.git(t, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			state, err := ReadGitState(r.dir)
			if err != nil {
				t.Fatal(err)
			}
			if state.Dirty != tt.dirty {
				t.Errorf("Dirty = %v, want %v", state.Dirty, tt.dirty)
			}
			if got := r.git(t, "rev-parse", "HEAD"); got != beforeHead {
				t.Errorf("reading state changed HEAD: %s -> %s", beforeHead, got)
			}
			if got := r.git(t, "status", "--porcelain=v1", "-z", "--untracked-files=all"); got != beforeStatus {
				t.Errorf("reading state changed files/index: %q -> %q", beforeStatus, got)
			}
		})
	}
}

func TestReadGitStateRejectsInvalidWorktrees(t *testing.T) {
	t.Run("non-repository", func(t *testing.T) {
		_, err := ReadGitState(t.TempDir())
		assertProjectGitError(t, err, "--dir")
	})
	t.Run("missing-directory", func(t *testing.T) {
		_, err := ReadGitState(filepath.Join(t.TempDir(), "does-not-exist"))
		assertProjectGitError(t, err, "existing directory")
	})
	t.Run("file-not-directory", func(t *testing.T) {
		r := newProjectGitTestRepo(t, true)
		_, err := ReadGitState(filepath.Join(r.dir, "tracked.txt"))
		assertProjectGitError(t, err, "not a directory")
	})
	t.Run("unborn-HEAD", func(t *testing.T) {
		r := newProjectGitTestRepo(t, false)
		_, err := ReadGitState(r.dir)
		assertProjectGitError(t, err, "initial commit")
	})
	t.Run("detached-HEAD", func(t *testing.T) {
		r := newProjectGitTestRepo(t, true)
		r.git(t, "checkout", "--detach")
		_, err := ReadGitState(r.dir)
		assertProjectGitError(t, err, "detached HEAD")
		assertProjectGitError(t, err, "switch")
	})
	t.Run("bare-repository", func(t *testing.T) {
		r := newProjectGitTestRepo(t, true)
		bare := filepath.Join(t.TempDir(), "bare.git")
		r.git(t, "clone", "--bare", "--", r.dir, bare)
		_, err := ReadGitState(bare)
		assertProjectGitError(t, err, "bare Git repository")
	})
	t.Run("metadata-not-worktree", func(t *testing.T) {
		r := newProjectGitTestRepo(t, true)
		_, err := ReadGitState(filepath.Join(r.dir, ".git"))
		assertProjectGitError(t, err, "not inside a Git worktree")
	})
}

func TestValidateCommitRange(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	base := r.git(t, "rev-parse", "HEAD")
	head := r.commit(t, "tracked.txt", "second\n")
	for _, commitRange := range []string{base + ".." + head, base[:8] + ".." + head[:8], strings.ToUpper(base[:12]) + ".." + strings.ToUpper(head[:12]), base + ".." + base} {
		gotBase, gotHead, err := ValidateCommitRange(r.dir, commitRange)
		if err != nil {
			t.Fatalf("ValidateCommitRange(%q): %v", commitRange, err)
		}
		wantHead := head
		if commitRange == base+".."+base {
			wantHead = base
		}
		if gotBase != base || gotHead != wantHead {
			t.Errorf("range = %s..%s, want %s..%s", gotBase, gotHead, base, wantHead)
		}
	}
	subdir := filepath.Join(r.dir, "nested")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateCommitRange(subdir, base[:8]+".."+head[:8]); err != nil {
		t.Errorf("range from subdirectory: %v", err)
	}
	assertInvalid := func(commitRange, contains string) {
		t.Helper()
		gotBase, gotHead, err := ValidateCommitRange(r.dir, commitRange)
		assertProjectGitError(t, err, contains)
		if gotBase != "" || gotHead != "" {
			t.Errorf("invalid range returned partial hashes: %s..%s", gotBase, gotHead)
		}
	}
	for _, commitRange := range []string{
		"", "HEAD..HEAD", "main.." + head, base + "..--all", "--help.." + head,
		base + "..." + head, base + ".." + head + ".." + head,
		" " + base + ".." + head, base + ".." + head + "\n", "abc..def",
		base + "^.." + head, base + ".." + head + ";touch /tmp/not-run",
		base + "..", ".." + head, strings.Repeat("a", 65) + ".." + head,
	} {
		assertInvalid(commitRange, "invalid commit range")
	}
	assertInvalid(strings.Repeat("0", 40)+".."+head, "does not exist")
	assertInvalid(head+".."+base, "not an ancestor")
	tree := r.git(t, "rev-parse", "HEAD^{tree}")
	assertInvalid(tree+".."+head, "not a commit")
	blob := r.git(t, "rev-parse", "HEAD:tracked.txt")
	assertInvalid(blob+".."+head, "not a commit")
	r.git(t, "tag", "-a", "annotated", "-m", "tag")
	tag := r.git(t, "rev-parse", "annotated")
	assertInvalid(tag+".."+head, "not a commit")
	// Hex-looking ref names are not object IDs and cannot redirect resolution.
	r.git(t, "branch", "deadbeef", head)
	assertInvalid("deadbeef.."+head, "does not exist")
	r.git(t, "branch", base[:8], head)
	gotBase, _, err := ValidateCommitRange(r.dir, base[:8]+".."+head)
	if err != nil || gotBase != base {
		t.Errorf("hex branch changed hash resolution: base=%s, err=%v", gotBase, err)
	}
	r.git(t, "checkout", "-b", "other", base)
	other := r.commit(t, "other.txt", "other history\n")
	assertInvalid(head+".."+other, "not an ancestor")
	if _, _, err := ValidateCommitRange(t.TempDir(), base+".."+head); err == nil {
		t.Error("expected range resolution in a non-repository to fail")
	}
}

func TestValidateGitProgress(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	base := r.git(t, "rev-parse", "HEAD")
	previous := r.commit(t, "tracked.txt", "second\n")
	current := r.commit(t, "tracked.txt", "third\n")
	for _, commits := range [][3]string{{base, base, base}, {base, previous, previous}, {base, previous, current}, {base[:8], previous[:8], current[:8]}} {
		if err := ValidateGitProgress(r.dir, commits[0], commits[1], commits[2]); err != nil {
			t.Errorf("valid progress %v: %v", commits, err)
		}
	}
	for _, commits := range [][3]string{{"HEAD", previous, current}, {base, "--all", current}, {base, previous, "main"}, {base, "", current}, {base, previous, strings.Repeat("0", 40)}} {
		if err := ValidateGitProgress(r.dir, commits[0], commits[1], commits[2]); err == nil {
			t.Errorf("invalid progress %v accepted", commits)
		}
	}
	assertProjectGitError(t, ValidateGitProgress(r.dir, current, previous, current), "recorded base")
	r.git(t, "reset", "--hard", base)
	// The previous object remains readable after reset, but is no longer on HEAD.
	if typ := r.git(t, "cat-file", "-t", previous); typ != "commit" {
		t.Fatalf("old commit unexpectedly disappeared: %q", typ)
	}
	assertProjectGitError(t, ValidateGitProgress(r.dir, base, previous, r.git(t, "rev-parse", "HEAD")), "reset or rewritten")
	rewritten := r.commit(t, "tracked.txt", "different second\n")
	assertProjectGitError(t, ValidateGitProgress(r.dir, base, previous, rewritten), "reset or rewritten")
	if err := ValidateGitProgress(r.dir, base, rewritten, rewritten); err != nil {
		t.Errorf("new explicit history: %v", err)
	}
}

func TestProjectGitIgnoresReplacementObjects(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	base := r.git(t, "rev-parse", "HEAD")
	previous := r.commit(t, "tracked.txt", "original\n")
	r.git(t, "reset", "--hard", base)
	current := r.commit(t, "tracked.txt", "rewritten\n")
	tree := r.git(t, "rev-parse", current+"^{tree}")
	// A replacement for current makes the old commit appear to be its parent
	// unless --no-replace-objects is used for every ancestry command.
	fake := r.git(t, "commit-tree", tree, "-p", previous, "-m", "fake ancestry")
	r.git(t, "replace", current, fake)
	assertProjectGitError(t, ValidateGitProgress(r.dir, base, previous, current), "reset or rewritten")
	_, _, err := ValidateCommitRange(r.dir, previous+".."+current)
	assertProjectGitError(t, err, "not an ancestor")
	state, err := ReadGitState(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Head != current {
		t.Errorf("HEAD = %s, want actual commit %s", state.Head, current)
	}
}

func TestProjectGitIgnoresAmbientEnvironmentAndHooks(t *testing.T) {
	r := newProjectGitTestRepo(t, true)
	base := r.git(t, "rev-parse", "HEAD")
	head := r.commit(t, "tracked.txt", "second\n")
	r.write(t, "tracked.txt", "unstaged\n")
	other := newProjectGitTestRepo(t, true)
	other.commit(t, "tracked.txt", "other repository\n")
	// Set an index whose clean contents would hide the real worktree's change.
	foreignIndex := filepath.Join(other.dir, ".git", "index")
	t.Setenv("GIT_DIR", filepath.Join(other.dir, ".git"))
	t.Setenv("GIT_COMMON_DIR", filepath.Join(other.dir, ".git"))
	t.Setenv("GIT_WORK_TREE", other.dir)
	t.Setenv("GIT_INDEX_FILE", foreignIndex)
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(other.dir, ".git", "objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(other.dir, ".git", "objects"))
	t.Setenv("GIT_NAMESPACE", "wrong-namespace")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(r.dir))
	t.Setenv("GIT_PREFIX", "wrong-prefix/")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_CONFIG_PARAMETERS", "invalid configuration")
	globalConfig := filepath.Join(t.TempDir(), "hostile-global-config")
	if err := os.WriteFile(globalConfig, []byte("[core]\n\tbare = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor-hook")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf ran > '"+marker+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.git(t, "config", "core.fsmonitor", hook)
	state, err := ReadGitState(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Directory != projectGitTestCanonical(t, r.dir) || state.Head != head || state.Branch != "main" || !state.Dirty {
		t.Errorf("ambient environment changed actual state: %+v", state)
	}
	gotBase, gotHead, err := ValidateCommitRange(r.dir, base[:8]+".."+head[:8])
	if err != nil || gotBase != base || gotHead != head {
		t.Errorf("ambient environment changed range: %s..%s, %v", gotBase, gotHead, err)
	}
	if err := ValidateGitProgress(r.dir, base, base, head); err != nil {
		t.Errorf("ambient environment changed progress: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("read-only Git commands executed a hook: %v", err)
	}
	// Environment variables must not turn an unrelated directory into a repo.
	_, err = ReadGitState(t.TempDir())
	assertProjectGitError(t, err, "--dir")
}
