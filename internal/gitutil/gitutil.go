// Package gitutil provides git repository helpers (backed by go-git, so no git
// binary is required) used to locate the store ($GIT_ROOT/.gitward.json) and
// the private base snapshot (.git/gitward/base.json), stage files, and test
// ignore rules.
package gitutil

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Open opens the repository containing dir (or the cwd when dir is empty),
// walking up to find the .git directory.
func Open(dir string) (*git.Repository, error) {
	if dir == "" {
		dir = "."
	}
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}
	return repo, nil
}

// Root returns the absolute worktree root for the repo containing dir.
func Root(dir string) (string, error) {
	repo, err := Open(dir)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	root := wt.Filesystem.Root()
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// GitDir resolves the absolute .git directory for the worktree at root,
// handling the common cases: a real .git directory, and a ".git" file
// containing "gitdir: <path>" (worktrees and submodules).
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
	return "", fmt.Errorf("could not parse gitdir from %s", dotgit)
}

// StorePath is the canonical location of the encrypted store.
func StorePath(root string) string { return filepath.Join(root, ".gitward.json") }

// BasePath is the canonical location of the private plaintext base snapshot,
// hidden inside .git so it is neither committed nor visible in the work tree.
func BasePath(gitDir string) string { return filepath.Join(gitDir, "gitward", "base.json") }

// EnsureDir creates the directory for path if missing.
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

// Stage adds a repo-relative path to the index.
func Stage(root, rel string) error {
	repo, err := Open(root)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	_, err = wt.Add(rel)
	return err
}

// PathIgnored reports whether the repo-relative path is ignored by git,
// evaluating .gitignore patterns via go-git's matcher.
func PathIgnored(root, rel string) bool {
	repo, err := Open(root)
	if err != nil {
		return false
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false
	}
	patterns, err := gitignore.ReadPatterns(wt.Filesystem, nil)
	if err != nil {
		return false
	}
	m := gitignore.NewMatcher(patterns)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return m.Match(parts, false)
}
