package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/taigrr/gitward/internal/store"
)

func tierOf(s string) store.Tier {
	if s == "runtime" {
		return store.Runtime
	}
	return store.Buildtime
}

// relToRoot converts a user-supplied path argument (interpreted relative to the
// current working directory, or absolute) into a path relative to the repo
// root, which is what the engine's scan functions expect. An empty arg stays
// empty (meaning "whole repo"). "." resolves to the current directory relative
// to root.
func relToRoot(root, arg string) (string, error) {
	if arg == "" {
		return "", nil
	}
	abs := arg
	if !filepath.IsAbs(abs) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		abs = filepath.Join(cwd, arg)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q is outside the repository root", arg)
	}
	return rel, nil
}
