package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewIgnoreCmd marks a variable as ward-ignored for a target/env/tier, or lists
// the current ignored keys when no key is given.
func NewIgnoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ignore <path> <env> <buildtime|runtime> [KEY]",
		Short: "Stop managing a variable (preserve it in the leaf, never capture it)",
		Args:  cobra.RangeArgs(3, 4),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			path, err := relToRoot(e.Root, args[0])
			if err != nil {
				return err
			}
			cell, err := engine.ParseCellArgs(path, args[1], args[2])
			if err != nil {
				return err
			}
			if len(args) == 3 {
				for _, k := range e.IgnoredKeys(cell) {
					c.Println(k)
				}
				return nil
			}
			if err := e.Ignore(cell, args[3]); err != nil {
				return err
			}
			c.Printf("ignoring %s in %s\n", args[3], cell)
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewUnignoreCmd clears a variable's ignore marker.
func NewUnignoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unignore <path> <env> <buildtime|runtime> <KEY>",
		Short: "Resume managing a previously ignored variable",
		Args:  cobra.ExactArgs(4),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			path, err := relToRoot(e.Root, args[0])
			if err != nil {
				return err
			}
			cell, err := engine.ParseCellArgs(path, args[1], args[2])
			if err != nil {
				return err
			}
			if err := e.Unignore(cell, args[3]); err != nil {
				return err
			}
			c.Printf("no longer ignoring %s in %s\n", args[3], cell)
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}
