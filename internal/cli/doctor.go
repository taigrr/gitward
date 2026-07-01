package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewDoctorCmd runs health checks.
func NewDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose store, hooks, recipients, and .gitignore coverage",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			checks := e.Doctor()
			allOK := true
			for _, ck := range checks {
				mark := "ok  "
				if !ck.OK {
					mark = "FAIL"
					allOK = false
				}
				c.Printf("[%s] %s — %s\n", mark, ck.Name, ck.Info)
			}
			if !allOK {
				return fmt.Errorf("doctor found problems")
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewHookCmd is the internal git-hook entrypoint.
func NewHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "hook <post-checkout|post-merge|pre-commit>",
		Short:  "Internal: git hook entrypoint",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runHook(c, args[0])
		},
	}
	return cmd
}

// runHook implements the hook behavior. It must never fail a git operation
// except pre-commit on an unresolved conflict.
func runHook(c *cobra.Command, name string) error {
	e, err := engine.Open("")
	if err != nil {
		// Not in a repo or git problem: do not block.
		fmt.Fprintf(os.Stderr, "ward: %v (skipping)\n", err)
		return nil
	}
	if !e.Initialized() {
		// No store or no key: warn but never block.
		fmt.Fprintln(os.Stderr, "ward: store unavailable (no key?) — leaf files not synced")
		return nil
	}

	plan, err := e.Plan()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ward: %v (skipping)\n", err)
		return nil
	}

	switch name {
	case "post-checkout", "post-merge":
		conflicts, err := e.Apply(plan, engine.StoreToLeaf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ward: %v (skipping)\n", err)
			return nil
		}
		if conflicts > 0 {
			fmt.Fprintf(os.Stderr, "ward: %d conflict(s) between store and local edits — run 'ward resolve'\n", conflicts)
		}
		return nil

	case "pre-commit":
		conflicts, err := e.Apply(plan, engine.LeafToStore)
		if err != nil {
			// A real error here should block rather than silently commit stale secrets.
			return err
		}
		if conflicts > 0 {
			return fmt.Errorf("ward: %d conflict(s) — run 'ward resolve' before committing", conflicts)
		}
		// Stage the (possibly updated) store so captured edits land in this commit.
		if err := e.StageStore(); err != nil {
			fmt.Fprintf(os.Stderr, "ward: could not stage store: %v\n", err)
		}
		return nil
	}
	return fmt.Errorf("unknown hook %q", name)
}
