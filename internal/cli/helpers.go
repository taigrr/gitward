package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
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

// cellArgs is the shared usage fragment for commands that address a cell.
const cellArgs = "(<leaf-file> | <path> <env> <buildtime|runtime>)"

// cellArgsHelp documents the two accepted cell addressing forms.
const cellArgsHelp = "A cell may be addressed either by its leaf file (e.g. apps/web/.env.production,\n" +
	"from which env and tier are derived) or by the explicit triple\n" +
	"<path> <env> <buildtime|runtime>, where env '_' means the suffix-less file."

// parseCellRef resolves a cell from the leading args, returning the cell and
// the remaining (unconsumed) args. A single leaf-file path consumes one arg;
// otherwise three args (path env tier) are consumed. Paths are interpreted
// relative to the current directory.
func parseCellRef(root string, args []string) (engine.Cell, []string, error) {
	if len(args) == 0 {
		return engine.Cell{}, nil, fmt.Errorf("missing cell: expected %s", cellArgs)
	}
	if engine.IsLeafFileName(args[0]) {
		rel, err := relToRoot(root, args[0])
		if err != nil {
			return engine.Cell{}, nil, err
		}
		cell, err := engine.ParseCellFile(rel)
		if err != nil {
			return engine.Cell{}, nil, err
		}
		return cell, args[1:], nil
	}
	if len(args) < 3 {
		return engine.Cell{}, nil, fmt.Errorf("expected %s, got %d argument(s)", cellArgs, len(args))
	}
	rel, err := relToRoot(root, args[0])
	if err != nil {
		return engine.Cell{}, nil, err
	}
	cell, err := engine.ParseCellArgs(rel, args[1], args[2])
	if err != nil {
		return engine.Cell{}, nil, err
	}
	return cell, args[3:], nil
}

// cellArgCount validates that args hold a cell reference followed by between
// min and max trailing arguments.
func cellArgCount(min, max int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("missing cell: expected %s", cellArgs)
		}
		consumed := 3
		if engine.IsLeafFileName(args[0]) {
			consumed = 1
		}
		rest := len(args) - consumed
		if rest < min || (max >= 0 && rest > max) {
			return fmt.Errorf("wrong number of arguments after %s", cellArgs)
		}
		return nil
	}
}
