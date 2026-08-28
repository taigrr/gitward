package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}

func TestRootFindsGitDirFromNestedPath(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "apps", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Root(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("Root() = %q, want %q", got, root)
	}
}

func TestGitDirResolvesDotGitFile(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git", "worktrees", "feature")
	writeTestFile(t, filepath.Join(root, ".git"), "gitdir: .git/worktrees/feature\n")

	got, err := GitDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != gitDir {
		t.Fatalf("GitDir() = %q, want %q", got, gitDir)
	}
}

func TestHooksDirHonorsCustomHooksPath(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "core.hooksPath", "custom-hooks")

	got, err := HooksDir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "custom-hooks")
	if got != want {
		t.Fatalf("HooksDir() = %q, want %q", got, want)
	}
}

func TestIgnoreMatcherMatchesNestedGitignoreRules(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".gitignore"), "*.env\n")
	writeTestFile(t, filepath.Join(root, "apps", ".gitignore"), "!keep.env\n*.local\n")

	matcher, err := NewIgnoreMatcher(root)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		rel  string
		want bool
	}{
		{"root.env", true},
		{"apps/api.env", true},
		{"apps/secret.local", true},
		{"apps/keep.env", false},
		{"README.md", false},
	}
	for _, c := range cases {
		if got := matcher.Match(c.rel); got != c.want {
			t.Errorf("Match(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}
