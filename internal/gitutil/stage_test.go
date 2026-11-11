package gitutil

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStageAddsOnlyRequestedPath(t *testing.T) {
	for _, name := range []string{".gitward.json", "-secret.json", "store with spaces.json"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init")
			writeTestFile(t, filepath.Join(root, name), "encrypted store")
			writeTestFile(t, filepath.Join(root, "unstaged.json"), "unrelated edit")
			if err := Stage(root, name); err != nil {
				t.Fatal(err)
			}
			output, err := gitOutput(root, "diff", "--cached", "--name-only", "-z")
			if err != nil {
				t.Fatal(err)
			}
			if output != name+"\x00" {
				t.Fatalf("staged paths = %q, want only %q", output, name)
			}
		})
	}
}

func TestStageReportsMissingPath(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	if err := Stage(root, "missing.json"); err == nil {
		t.Fatal("Stage succeeded for a missing path")
	}
}

func TestStageInLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, root, "worktree", "add", "-b", "feature", worktree)
	writeTestFile(t, filepath.Join(worktree, ".gitward.json"), "encrypted store")
	if err := Stage(worktree, ".gitward.json"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ dir, want string }{{root, ""}, {worktree, ".gitward.json\x00"}} {
		output, err := gitOutput(test.dir, "diff", "--cached", "--name-only", "-z")
		if err != nil {
			t.Fatal(err)
		}
		if output != test.want {
			t.Errorf("index in %s = %q, want %q", test.dir, output, test.want)
		}
	}
}
