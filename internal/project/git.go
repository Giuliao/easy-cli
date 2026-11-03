package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitState describes the actual, canonical worktree rather than an agent session.
type GitState struct {
	Directory    string `json:"directory"`
	GitCommonDir string `json:"git_common_dir"`
	Repository   string `json:"repository"`
	Branch       string `json:"branch"`
	Head         string `json:"head"`
	Dirty        bool   `json:"dirty"`
}

// ReadGitState reads a non-bare worktree with a committed, attached HEAD.
// directory may be a subdirectory or a symlink into the worktree. The context
// bookkeeping file alone does not make the worktree dirty; no Git files change.
func ReadGitState(directory string) (GitState, error) {
	dir, err := canonicalGitDirectory(directory)
	if err != nil {
		return GitState{}, err
	}
	bare, err := runProjectGit(dir, "rev-parse", "--is-bare-repository")
	if err != nil {
		return GitState{}, fmt.Errorf("inspect Git repository at %q: select an existing Git worktree with --dir: %w", dir, err)
	}
	if gitOutputLine(bare) == "true" {
		return GitState{}, fmt.Errorf("%q is a bare Git repository; select a non-bare worktree with --dir", dir)
	}
	inside, err := runProjectGit(dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return GitState{}, fmt.Errorf("inspect Git worktree at %q: %w", dir, err)
	}
	if gitOutputLine(inside) != "true" {
		return GitState{}, fmt.Errorf("%q is not inside a Git worktree; select a worktree directory with --dir", dir)
	}
	rootOutput, err := runProjectGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return GitState{}, fmt.Errorf("find Git worktree root: %w", err)
	}
	root, err := canonicalGitDirectory(gitOutputLine(rootOutput))
	if err != nil {
		return GitState{}, fmt.Errorf("resolve Git worktree root: %w", err)
	}
	commonOutput, err := runProjectGit(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return GitState{}, fmt.Errorf("find Git common directory: %w", err)
	}
	common, err := canonicalGitDirectory(gitOutputLine(commonOutput))
	if err != nil {
		return GitState{}, fmt.Errorf("resolve Git common directory: %w", err)
	}

	branchOutput, err := runProjectGit(root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		if projectGitExitCode(err, 1) {
			return GitState{}, fmt.Errorf("%q has detached HEAD; switch to the intended branch before resuming work", root)
		}
		return GitState{}, fmt.Errorf("read Git branch; restore a valid attached HEAD: %w", err)
	}
	branchRef := gitOutputLine(branchOutput)
	if !strings.HasPrefix(branchRef, "refs/heads/") {
		return GitState{}, fmt.Errorf("HEAD at %q does not point to a local branch; switch to the intended branch before resuming work", root)
	}
	headOutput, err := runProjectGit(root, "rev-parse", "--verify", "--end-of-options", "HEAD^{commit}")
	if err != nil {
		return GitState{}, fmt.Errorf("%q has no valid HEAD commit (empty repository or unborn HEAD); create an initial commit or repair HEAD before resuming work: %w", root, err)
	}

	gitDirOutput, err := runProjectGit(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return GitState{}, fmt.Errorf("find Git worktree metadata directory: %w", err)
	}
	gitDir, err := canonicalGitDirectory(gitOutputLine(gitDirOutput))
	if err != nil {
		return GitState{}, fmt.Errorf("resolve Git worktree metadata directory: %w", err)
	}
	mainRoot := root
	if gitDir != common {
		// A normal common dir's .git parent identifies the main worktree.
		// For other layouts inspect Git's main worktree entry, but do not
		// silently use a relocated metadata directory as a repository name.
		commonPath := gitOutputLine(commonOutput)
		if filepath.Base(commonPath) == ".git" {
			mainRoot, err = canonicalGitDirectory(filepath.Dir(commonPath))
		} else {
			var worktrees []byte
			worktrees, err = runProjectGit(root, "worktree", "list", "--porcelain", "-z")
			if err == nil {
				entry, _, found := bytes.Cut(worktrees, []byte{0})
				if !found || !bytes.HasPrefix(entry, []byte("worktree ")) {
					err = fmt.Errorf("Git did not report a main worktree; repair the worktree metadata before resuming work")
				} else {
					mainRoot, err = canonicalGitDirectory(string(entry[len("worktree "):]))
				}
			}
		}
		if err != nil {
			return GitState{}, fmt.Errorf("resolve main Git worktree: %w", err)
		}
		if mainRoot == common {
			return GitState{}, fmt.Errorf("cannot identify the main worktree from relocated Git common directory %q; use a standard .git layout or select the main worktree with --dir", common)
		}
	}
	status, err := runProjectGit(root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return GitState{}, fmt.Errorf("read Git dirty state: %w", err)
	}
	dirty, err := projectGitStatusDirty(status)
	if err != nil {
		return GitState{}, err
	}
	return GitState{
		Directory:    root,
		GitCommonDir: common,
		Repository:   filepath.Base(mainRoot),
		Branch:       strings.TrimPrefix(branchRef, "refs/heads/"),
		Head:         gitOutputLine(headOutput),
		Dirty:        dirty,
	}, nil
}

// ValidateCommitRange accepts only base..head made of hexadecimal commit IDs
// (at least four characters, Git's minimum abbreviation length). It resolves
// unique object prefixes to full commit hashes and requires forward ancestry.
// An empty range is invalid; the caller chooses how to initialize an empty range.
func ValidateCommitRange(directory, commitRange string) (base string, head string, err error) {
	parts := strings.Split(commitRange, "..")
	if len(parts) != 2 || !projectGitHexID(parts[0]) || !projectGitHexID(parts[1]) {
		return "", "", fmt.Errorf("invalid commit range %q; use base..head with hexadecimal commit IDs (at least 4 characters), not refs or options", commitRange)
	}
	dir, err := canonicalGitDirectory(directory)
	if err != nil {
		return "", "", err
	}
	base, err = resolveProjectGitCommit(dir, parts[0], "base")
	if err != nil {
		return "", "", err
	}
	head, err = resolveProjectGitCommit(dir, parts[1], "head")
	if err != nil {
		return "", "", err
	}
	ancestor, err := projectGitAncestor(dir, base, head)
	if err != nil {
		return "", "", err
	}
	if !ancestor {
		return "", "", fmt.Errorf("invalid commit range: base %s is not an ancestor of head %s; select a forward range on the intended history", base, head)
	}
	return base, head, nil
}

// ValidateGitProgress checks both the recorded range and forward progress. A
// previous commit that still exists after a reset or rebase is not sufficient:
// it must remain an ancestor of the current commit. Unchanged HEAD is valid.
func ValidateGitProgress(directory, base, previousHead, currentHead string) error {
	dir, err := canonicalGitDirectory(directory)
	if err != nil {
		return err
	}
	base, err = resolveProjectGitCommit(dir, base, "recorded base")
	if err != nil {
		return err
	}
	previousHead, err = resolveProjectGitCommit(dir, previousHead, "previous head")
	if err != nil {
		return err
	}
	currentHead, err = resolveProjectGitCommit(dir, currentHead, "current head")
	if err != nil {
		return err
	}
	ancestor, err := projectGitAncestor(dir, base, previousHead)
	if err != nil {
		return err
	}
	if !ancestor {
		return fmt.Errorf("recorded base %s is not an ancestor of previous head %s; verify the recorded range and restore its history or explicitly start a new task", base, previousHead)
	}
	ancestor, err = projectGitAncestor(dir, previousHead, currentHead)
	if err != nil {
		return err
	}
	if !ancestor {
		return fmt.Errorf("previous head %s is not an ancestor of current head %s; history was reset or rewritten: restore the recorded history or explicitly start a new task", previousHead, currentHead)
	}
	return nil
}

func canonicalGitDirectory(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve Git directory %q; select an existing directory with --dir: %w", directory, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve Git directory %q; select an existing directory with --dir: %w", directory, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect Git directory %q: %w", canonical, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory; select a Git worktree directory with --dir", canonical)
	}
	return canonical, nil
}

func runProjectGit(directory string, args ...string) ([]byte, error) {
	gitArgs := []string{
		"--no-replace-objects",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "core.untrackedCache=false",
		"-C", directory,
	}
	cmd := exec.Command("git", append(gitArgs, args...)...)
	// Repo selection, alternate object stores, namespaces, replacement refs,
	// index overrides and injected configuration must not leak from the agent's
	// environment. Global/system config and hooks are unnecessary for these
	// read-only commands; repository config (including worktree layout) remains.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") && key != "LC_ALL" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), detail, err)
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func gitOutputLine(output []byte) string {
	// Trim only Git's terminator, not whitespace that can belong to a path.
	return strings.TrimSuffix(string(output), "\n")
}

func projectGitExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}

func projectGitHexID(id string) bool {
	if len(id) < 4 || len(id) > 64 {
		return false
	}
	for _, char := range id {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func resolveProjectGitCommit(directory, id, label string) (string, error) {
	if !projectGitHexID(id) {
		return "", fmt.Errorf("invalid %s commit ID %q; use a hexadecimal commit hash (at least 4 characters), not a ref or option", label, id)
	}
	// --disambiguate resolves object prefixes, never a hex-looking branch/tag
	// name. Peeling with ^{commit} would incorrectly accept annotated tag IDs.
	output, err := runProjectGit(directory, "rev-parse", "--disambiguate="+strings.ToLower(id))
	if err != nil {
		return "", fmt.Errorf("resolve %s commit %q; select the correct repository and a valid commit hash: %w", label, id, err)
	}
	hashes := strings.Fields(string(output))
	if len(hashes) == 0 {
		return "", fmt.Errorf("%s commit %q does not exist in this repository; verify the hash and directory or restore the recorded history", label, id)
	}
	if len(hashes) != 1 {
		return "", fmt.Errorf("%s commit ID %q is ambiguous; use a longer hexadecimal commit hash", label, id)
	}
	objectType, err := runProjectGit(directory, "cat-file", "-t", hashes[0])
	if err != nil {
		return "", fmt.Errorf("inspect %s commit %q: %w", label, id, err)
	}
	if gitOutputLine(objectType) != "commit" {
		return "", fmt.Errorf("%s ID %q identifies a %s, not a commit; use the commit's hexadecimal hash", label, id, gitOutputLine(objectType))
	}
	return hashes[0], nil
}

func projectGitAncestor(directory, base, head string) (bool, error) {
	_, err := runProjectGit(directory, "merge-base", "--is-ancestor", base, head)
	if err == nil {
		return true, nil
	}
	if projectGitExitCode(err, 1) {
		return false, nil
	}
	return false, fmt.Errorf("verify Git commit ancestry; check the repository's history: %w", err)
}

func projectGitStatusDirty(status []byte) (bool, error) {
	dirty := false
	for len(status) > 0 {
		entry, remaining, found := bytes.Cut(status, []byte{0})
		if !found || len(entry) < 4 || entry[2] != ' ' {
			return false, fmt.Errorf("Git returned invalid porcelain status; cannot safely determine dirty state")
		}
		status = remaining
		if string(entry[3:]) != ".easy-cli/context" {
			dirty = true
		}
		// In -z format a rename/copy lists destination first, then source.
		// Renaming another file into context (or context out) is real work.
		if entry[0] == 'R' || entry[1] == 'R' || entry[0] == 'C' || entry[1] == 'C' {
			source, remaining, found := bytes.Cut(status, []byte{0})
			if !found || len(source) == 0 {
				return false, fmt.Errorf("Git returned invalid rename status; cannot safely determine dirty state")
			}
			status = remaining
			if string(source) != ".easy-cli/context" {
				dirty = true
			}
		}
	}
	return dirty, nil
}
