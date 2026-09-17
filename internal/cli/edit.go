package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
	"github.com/taigrr/gitward/internal/leaf"
	"github.com/taigrr/gitward/internal/merge"
)

// NewEditCmd opens a cell's decrypted values in $EDITOR and captures the result.
func NewEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit " + cellArgs,
		Short: "Edit a target's secrets in $EDITOR, capturing back to the store",
		Long: "Open the cell's decrypted values in $VISUAL / $EDITOR and write the result\n" +
			"back to the store, leaf file, and base snapshot.\n\n" + cellArgsHelp + "\n\n" +
			"For non-interactive use prefer 'ward set' / 'ward unset'.",
		Args: cellArgCount(0, 0),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			cell, _, err := parseCellRef(e.Root, args)
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

// Values accepted by resolve --take.
const (
	takeStore = "store"
	takeLeaf  = "leaf"
)

// resolvedJSON is one resolved key in `resolve --json`.
type resolvedJSON struct {
	cellJSON
	Key        string `json:"key"`
	Resolution string `json:"resolution"`
}

// NewResolveCmd resolves conflicting keys, interactively or via flags.
func NewResolveCmd() *cobra.Command {
	var (
		take     string
		key      string
		value    string
		valueSet bool
		del      bool
	)
	cmd := &cobra.Command{
		Use:   "resolve [path] [--take store|leaf] [--key KEY (--value V | --delete)]",
		Short: "Resolve conflicting keys (interactively, or non-interactively via flags)",
		Long: "Resolve keys where both the store and the leaf file changed since the last\n" +
			"sync. Without flags, each conflict is presented on the terminal for a choice.\n\n" +
			"Non-interactive forms (for scripts and agents):\n" +
			"  --take store|leaf        resolve every matching conflict toward one side\n" +
			"  --key K --value V        resolve conflict on K to an explicit value\n" +
			"  --key K --delete         resolve conflict on K by deleting it\n" +
			"  --key K --take store     resolve only K toward one side\n\n" +
			"An optional path (directory or leaf file) limits which cells are considered.\n" +
			"Running without flags on a non-terminal stdin is an error.\n\n" +
			"With --json, emits {resolved, remaining, keys:[{path, env, tier, file, key,\n" +
			"resolution}]} where resolution is store, leaf, delete, or value.",
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(c *cobra.Command, _ []string) error {
			valueSet = c.Flags().Changed("value")
			if take != "" && take != takeStore && take != takeLeaf {
				return fmt.Errorf("--take must be %q or %q", takeStore, takeLeaf)
			}
			if (valueSet || del) && key == "" {
				return fmt.Errorf("--value/--delete require --key")
			}
			if valueSet && del {
				return fmt.Errorf("--value and --delete are mutually exclusive")
			}
			if take != "" && (valueSet || del) {
				return fmt.Errorf("--take cannot be combined with --value/--delete")
			}
			if key != "" && take == "" && !valueSet && !del {
				return fmt.Errorf("--key requires one of --take, --value, or --delete")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			filter, err := pathFilter(e.Root, args)
			if err != nil {
				return err
			}
			interactive := take == "" && key == ""
			if interactive && (jsonFlag || !stdinIsTerminal()) {
				return fmt.Errorf("no terminal for interactive resolution; use --take or --key")
			}

			var resolved []resolvedJSON
			remaining := 0
			for _, cr := range plan {
				if filter != "" && cr.Cell.Path != filter {
					continue
				}
				for _, r := range cr.Results {
					if !r.Conflict() {
						continue
					}
					if key != "" && r.Key != key {
						remaining++
						continue
					}
					var resolution string
					switch {
					case take == takeStore:
						resolution, err = takeStore, e.ResolveKey(cr.Cell, r.Key, r.Store.Value(), false)
					case take == takeLeaf:
						resolution, err = takeLeaf, e.ResolveKey(cr.Cell, r.Key, r.Leaf.Value(), false)
					case del:
						resolution, err = opDelete, e.ResolveKey(cr.Cell, r.Key, "", true)
					case valueSet:
						resolution, err = "value", e.ResolveKey(cr.Cell, r.Key, value, false)
					default:
						resolution, err = promptResolve(c, e, cr.Cell, r)
					}
					if err != nil {
						return err
					}
					resolved = append(resolved, resolvedJSON{cellJSON: toCellJSON(cr.Cell), Key: r.Key, Resolution: resolution})
				}
			}
			if key != "" && len(resolved) == 0 {
				return fmt.Errorf("no conflict on key %q", key)
			}
			if jsonFlag {
				if resolved == nil {
					resolved = []resolvedJSON{}
				}
				return printJSON(c, struct {
					Resolved  int            `json:"resolved"`
					Remaining int            `json:"remaining"`
					Keys      []resolvedJSON `json:"keys"`
				}{len(resolved), remaining, resolved})
			}
			if interactive {
				c.Println()
			}
			c.Printf("resolved %d conflict(s)\n", len(resolved))
			return nil
		},
	}
	cmd.Flags().StringVar(&take, "take", "", "resolve toward one side: store or leaf")
	cmd.Flags().StringVar(&key, "key", "", "restrict to a single key")
	cmd.Flags().StringVar(&value, "value", "", "explicit value for --key")
	cmd.Flags().BoolVar(&del, "delete", false, "delete --key on both sides")
	addSSHFlag(cmd)
	return cmd
}

// promptResolve asks the user how to resolve one conflicting key and applies
// the answer, returning the resolution name. Output format is unchanged from
// earlier releases.
func promptResolve(c *cobra.Command, e *engine.Engine, cell engine.Cell, r merge.Result) (string, error) {
	c.Printf("\nconflict %s key %s\n", cell, r.Key)
	c.Printf("  [s] store: %q\n", r.Store.Value())
	c.Printf("  [l] leaf:  %q\n", r.Leaf.Value())
	choice, err := promptLine("  choose (s/l/type new value/d=delete): ")
	if err != nil {
		return "", err
	}
	switch choice {
	case "s":
		return takeStore, e.ResolveKey(cell, r.Key, r.Store.Value(), false)
	case "l":
		return takeLeaf, e.ResolveKey(cell, r.Key, r.Leaf.Value(), false)
	case "d":
		return opDelete, e.ResolveKey(cell, r.Key, "", true)
	default:
		return "value", e.ResolveKey(cell, r.Key, choice, false)
	}
}
