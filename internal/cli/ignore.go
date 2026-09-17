package cli

import (
	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewIgnoreCmd marks a variable as ward-ignored for a target/env/tier, or lists
// the current ignored keys when no key is given.
func NewIgnoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ignore " + cellArgs + " [KEY]",
		Short: "Stop managing a variable (preserve it in the leaf, never capture it)",
		Long: "Mark KEY as ward-ignored in the given cell: it stays in the leaf file (in a\n" +
			"trailing block) but is never captured into the store or reported as drift.\n" +
			"Without KEY, list the cell's ignored keys.\n\n" + cellArgsHelp + "\n\n" +
			"With --json, emits {path, env, tier, file, ignored:[...]}.",
		Args: cellArgCount(0, 1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			cell, rest, err := parseCellRef(e.Root, args)
			if err != nil {
				return err
			}
			if len(rest) == 1 {
				if err := e.Ignore(cell, rest[0]); err != nil {
					return err
				}
				if !jsonFlag {
					c.Printf("ignoring %s in %s\n", rest[0], cell)
					return nil
				}
			}
			ignored := e.IgnoredKeys(cell)
			if jsonFlag {
				if ignored == nil {
					ignored = []string{}
				}
				return printJSON(c, struct {
					cellJSON
					Ignored []string `json:"ignored"`
				}{toCellJSON(cell), ignored})
			}
			for _, k := range ignored {
				c.Println(k)
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewUnignoreCmd clears a variable's ignore marker.
func NewUnignoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unignore " + cellArgs + " <KEY>",
		Short: "Resume managing a previously ignored variable",
		Long:  "Clear KEY's ignore marker so it is captured into the store on the next sync.\n\n" + cellArgsHelp,
		Args:  cellArgCount(1, 1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			cell, rest, err := parseCellRef(e.Root, args)
			if err != nil {
				return err
			}
			if err := e.Unignore(cell, rest[0]); err != nil {
				return err
			}
			if jsonFlag {
				return printJSON(c, struct {
					cellJSON
					Key string `json:"key"`
				}{toCellJSON(cell), rest[0]})
			}
			c.Printf("no longer ignoring %s in %s\n", rest[0], cell)
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}
