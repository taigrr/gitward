package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
	"github.com/taigrr/gitward/internal/leaf"
)

// NewEditCmd opens a cell's decrypted values in $EDITOR and captures the result.
func NewEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <path> <env> <buildtime|runtime>",
		Short: "Edit a target's secrets in $EDITOR, capturing back to the store",
		Args:  cobra.ExactArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			cell, err := engine.ParseCellArgs(args[0], args[1], args[2])
			if err != nil {
				return err
			}
			vals, err := e.CellPlaintext(cell)
			if err != nil {
				return err
			}

			tmp, err := os.CreateTemp("", "ward-edit-*.env")
			if err != nil {
				return err
			}
			tmpPath := tmp.Name()
			defer os.Remove(tmpPath)
			if _, err := tmp.WriteString(leaf.Serialize(vals)); err != nil {
				tmp.Close()
				return err
			}
			tmp.Close()

			if err := runEditor(tmpPath); err != nil {
				return err
			}
			edited, err := os.ReadFile(tmpPath)
			if err != nil {
				return err
			}
			newVals, err := leaf.Parse(string(edited))
			if err != nil {
				return fmt.Errorf("parsing edited content: %w", err)
			}
			if err := e.SetCellPlaintext(cell, newVals); err != nil {
				return err
			}
			c.Printf("updated %s (%d key(s))\n", cell, len(newVals))
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

func runEditor(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// NewResolveCmd interactively resolves conflicting keys.
func NewResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve [path]",
		Short: "Interactively resolve conflicting keys",
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return fmt.Errorf("store not initialized (run 'ward init')")
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			resolved := 0
			for _, cr := range plan {
				if filter != "" && cr.Cell.Path != filter {
					continue
				}
				for _, r := range cr.Results {
					if !r.Conflict() {
						continue
					}
					c.Printf("\nconflict %s key %s\n", cr.Cell, r.Key)
					c.Printf("  [s] store: %q\n", r.Store.Value())
					c.Printf("  [l] leaf:  %q\n", r.Leaf.Value())
					choice, err := promptLine("  choose (s/l/type new value/d=delete): ")
					if err != nil {
						return err
					}
					switch choice {
					case "s":
						err = e.ResolveKey(cr.Cell, r.Key, r.Store.Value(), false)
					case "l":
						err = e.ResolveKey(cr.Cell, r.Key, r.Leaf.Value(), false)
					case "d":
						err = e.ResolveKey(cr.Cell, r.Key, "", true)
					default:
						err = e.ResolveKey(cr.Cell, r.Key, choice, false)
					}
					if err != nil {
						return err
					}
					resolved++
				}
			}
			c.Printf("\nresolved %d conflict(s)\n", resolved)
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}
