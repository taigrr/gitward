package gitutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// gitAdd shells out to `git add -- <rel>` in root. Used only from the
// pre-commit hook path, where the git binary is inherently available.
func gitAdd(root, rel string) error {
	cmd := exec.Command("git", "add", "--", rel)
	cmd.Dir = root
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add %s: %w: %s", rel, err, strings.TrimSpace(errb.String()))
	}
	return nil
}

// gitOutput runs a git command in root and returns its stdout.
func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
