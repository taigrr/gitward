// Package gitutil provides git repository helpers used to locate the store
// ($GIT_ROOT/.gitward.json) and the private base snapshot
// (.git/gitward/base.json), stage files, and test ignore rules.
//
// Repository discovery is done by walking the filesystem for the .git entry
// rather than via go-git's PlainOpen. This deliberately avoids parsing the
// repository config, so repos that declare extensions go-git does not
// understand (notably extensions.worktreeConfig, set by `git worktree` and
// `git maintenance`) still work.
package gitutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Root returns the absolute worktree root for the repo containing dir (or the
// cwd when dir is empty) by walking up until a .git entry (directory or file)
// is found.
func Root(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("not inside a git repository (no .git found from %s)", dir)
		}
		abs = parent
	}
}

// GitDir resolves the absolute git directory for the worktree at root. It
// handles the common cases: a real .git directory; and a ".git" file
// containing "gitdir: <path>" (linked worktrees and submodules), where the
// resolved path is the per-worktree git dir (e.g. .git/worktrees/<name>).
func GitDir(root string) (string, error) {
	dotgit := filepath.Join(root, ".git")
	info, err := os.Stat(dotgit)
	if err != nil {
		return "", fmt.Errorf("resolving git dir: %w", err)
	}
	if info.IsDir() {
		return dotgit, nil
	}
	// .git is a file: "gitdir: <path>"
	f, err := os.Open(dotgit)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "gitdir:"); ok {
			p := strings.TrimSpace(rest)
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			return filepath.Clean(p), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("could not parse gitdir from %s", dotgit)
}

// StorePath is the canonical location of the encrypted store.
func StorePath(root string) string { return filepath.Join(root, ".gitward.json") }

// BasePath is the canonical location of the private plaintext base snapshot,
// hidden inside the git dir so it is neither committed nor visible in the work
// tree. In a linked worktree this is the per-worktree git dir, giving each
// worktree its own base snapshot (matching its own checkout).
func BasePath(gitDir string) string { return filepath.Join(gitDir, "gitward", "base.json") }

// EnsureDir creates the directory for path if missing.
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

// HooksDir returns the absolute directory git runs hooks from for the worktree
// at root. It honors core.hooksPath (used by husky and similar tooling) by
// asking git directly (`git rev-parse --git-path hooks`), which also resolves
// correctly inside linked worktrees. If git is unavailable it falls back to
// the conventional <gitDir>/hooks.
func HooksDir(root string) (string, error) {
	out, err := gitOutput(root, "rev-parse", "--git-path", "hooks")
	if err != nil {
		gitDir, gerr := GitDir(root)
		if gerr != nil {
			return "", err
		}
		return filepath.Join(gitDir, "hooks"), nil
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return filepath.Clean(p), nil
}

// Stage adds a repo-relative path to the index by invoking the git binary.
// Staging only happens from within the pre-commit hook (i.e. during a git
// invocation), so the git binary is guaranteed to be present here; using it
// sidesteps go-git's config-extension validation.
func Stage(root, rel string) error {
	return gitAdd(root, rel)
}

// PathIgnored reports whether the repo-relative path is ignored by git,
// evaluating .gitignore patterns via go-git's matcher. It reads patterns
// directly off the filesystem rooted at root, without opening the repository,
// so it is unaffected by unsupported config extensions.
//
// Reading the patterns recursively walks the worktree, so callers that test
// many paths against the same root should build an IgnoreMatcher once with
// NewIgnoreMatcher and reuse it instead of calling PathIgnored in a loop.
func PathIgnored(root, rel string) bool {
	m, err := NewIgnoreMatcher(root)
	if err != nil {
		return false
	}
	return m.Match(rel)
}

// IgnoreMatcher evaluates .gitignore rules for a repo without re-reading the
// worktree on every check. Build it once with NewIgnoreMatcher and call Match
// for each candidate path.
type IgnoreMatcher struct {
	m gitignore.Matcher
}

// NewIgnoreMatcher reads all .gitignore patterns under root once (the
// expensive step) and returns a reusable matcher.
func NewIgnoreMatcher(root string) (*IgnoreMatcher, error) {
	fs := osfs.New(root)
	patterns, err := gitignore.ReadPatterns(fs, nil)
	if err != nil {
		return nil, err
	}
	return &IgnoreMatcher{m: gitignore.NewMatcher(patterns)}, nil
}

// Match reports whether the repo-relative path is ignored.
func (im *IgnoreMatcher) Match(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return im.m.Match(parts, false)
}
