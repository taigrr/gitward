package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookNames are the git hooks ward manages.
var hookNames = []string{"post-checkout", "post-merge", "pre-commit"}

const hookMarker = "# gitward-managed"

// InstallHooks writes (or updates) the git hooks that keep leaf files and the
// store in sync. Each hook simply invokes `ward hook <name>`. Installation is
// idempotent and preserves any pre-existing non-ward hook body by appending the
// ward invocation, so it is safe to run from a bun postinstall.
//
// The hooks never fail the git operation: `ward hook` exits 0 on recoverable
// problems (missing key, uninitialized store) after printing a warning, except
// pre-commit which exits non-zero only on an unresolved conflict.
func (e *Engine) InstallHooks() error {
	hooksDir := filepath.Join(e.GitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	for _, name := range hookNames {
		path := filepath.Join(hooksDir, name)
		if err := installOneHook(path, name); err != nil {
			return fmt.Errorf("installing %s: %w", name, err)
		}
	}
	return nil
}

func installOneHook(path, name string) error {
	invocation := fmt.Sprintf("%s\nward hook %s || exit $?\n", hookMarker, name)

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if strings.Contains(string(existing), hookMarker) {
			return nil // already installed
		}
		// Append to an existing hook, keeping its shebang and body.
		body := string(existing)
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += invocation
		return os.WriteFile(path, []byte(body), 0o755)
	}
	// Fresh hook.
	content := "#!/bin/sh\n" + invocation
	return os.WriteFile(path, []byte(content), 0o755)
}
