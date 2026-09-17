package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/taigrr/gitward/internal/engine"
)

// NewGetCmd prints one value (or a whole cell) without an editor.
func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get " + cellArgs + " [KEY]",
		Short: "Print a secret value (or all values of a cell) from the store",
		Long: "Print the store's value for KEY in the given cell. With no KEY, print every\n" +
			"key of the cell in dotenv form. The store is the source of truth here; use\n" +
			"'ward diff' to see whether the leaf file differs.\n\n" +
			cellArgsHelp + "\n\n" +
			"With --json, emits {path, env, tier, file, key, value} for one key or\n" +
			"{path, env, tier, file, values:{KEY: value}} for a cell. Exits 1 when the key\n" +
			"is not present.",
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
			vals, err := e.CellPlaintext(cell)
			if err != nil {
				return err
			}
			if len(rest) == 1 {
				key := rest[0]
				v, ok := vals[key]
				if !ok {
					return fmt.Errorf("key %q not found in %s", key, cell)
				}
				if jsonFlag {
					return printJSON(c, struct {
						cellJSON
						Key   string `json:"key"`
						Value string `json:"value"`
					}{toCellJSON(cell), key, v})
				}
				c.Println(v)
				return nil
			}
			if jsonFlag {
				return printJSON(c, struct {
					cellJSON
					Values map[string]string `json:"values"`
				}{toCellJSON(cell), vals})
			}
			keys := make([]string, 0, len(vals))
			for k := range vals {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				c.Printf("%s=%s\n", k, vals[k])
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewSetCmd writes one or more values without an editor.
func NewSetCmd() *cobra.Command {
	var (
		fromStdin bool
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "set " + cellArgs + " KEY=VALUE... | KEY --stdin",
		Short: "Set secret values in the store (and the leaf file) non-interactively",
		Long: "Set KEY to VALUE in the given cell, re-encrypting the store, regenerating the\n" +
			"leaf file, and advancing the base so the write is not seen as drift. Several\n" +
			"KEY=VALUE pairs may be given. With a bare KEY and --stdin, the value is read\n" +
			"from standard input (one trailing newline is stripped), which keeps secrets\n" +
			"out of shell history and process listings.\n\n" +
			cellArgsHelp + "\n\n" +
			"Refuses to run when the cell has unsynced changes (see 'ward status') unless\n" +
			"--force is given, because the write replaces the whole leaf file.\n\n" +
			"With --json, emits {path, env, tier, file, keys:[...]}.",
		Args: cellArgCount(1, -1),
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
			kv, err := parseAssignments(rest, fromStdin, c.InOrStdin())
			if err != nil {
				return err
			}
			if !force {
				if err := guardCellClean(e, cell); err != nil {
					return err
				}
			}
			if err := e.SetKeys(cell, kv); err != nil {
				return err
			}
			keys := make([]string, 0, len(kv))
			for k := range kv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if jsonFlag {
				return printJSON(c, struct {
					cellJSON
					Keys []string `json:"keys"`
				}{toCellJSON(cell), keys})
			}
			c.Printf("set %s in %s\n", strings.Join(keys, ", "), cell)
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read the value for a single bare KEY from stdin")
	cmd.Flags().BoolVar(&force, "force", false, "write even if the cell has unsynced changes")
	addSSHFlag(cmd)
	return cmd
}

// NewUnsetCmd removes keys without an editor.
func NewUnsetCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "unset " + cellArgs + " KEY...",
		Short: "Remove secret keys from the store (and the leaf file) non-interactively",
		Long: "Delete KEY(s) from the given cell in the store, the leaf file, and the base\n" +
			"snapshot. Missing keys are not an error.\n\n" +
			cellArgsHelp + "\n\n" +
			"Refuses to run when the cell has unsynced changes unless --force is given.\n\n" +
			"With --json, emits {path, env, tier, file, removed}.",
		Args: cellArgCount(1, -1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			cell, keys, err := parseCellRef(e.Root, args)
			if err != nil {
				return err
			}
			if !force {
				if err := guardCellClean(e, cell); err != nil {
					return err
				}
			}
			removed, err := e.UnsetKeys(cell, keys)
			if err != nil {
				return err
			}
			if jsonFlag {
				return printJSON(c, struct {
					cellJSON
					Removed int `json:"removed"`
				}{toCellJSON(cell), removed})
			}
			c.Printf("removed %d key(s) from %s\n", removed, cell)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "write even if the cell has unsynced changes")
	addSSHFlag(cmd)
	return cmd
}

// guardCellClean errors when the cell has pending local edits or conflicts,
// since a direct write would replace the leaf file and could discard them.
// Incoming-only cells (e.g. a fresh clone whose leaf is not yet generated) are
// allowed, because the write reads current values from the store anyway.
func guardCellClean(e *engine.Engine, cell engine.Cell) error {
	plan, err := e.Plan()
	if err != nil {
		return err
	}
	if engine.CellHasLocalChanges(plan, cell) {
		return fmt.Errorf("%s has unsynced local changes; run 'ward sync' (or 'ward resolve') first, or pass --force", cell)
	}
	return nil
}

// parseAssignments turns KEY=VALUE args (or a single bare KEY with stdin) into
// a map. Only the first '=' separates key from value.
func parseAssignments(args []string, fromStdin bool, in io.Reader) (map[string]string, error) {
	kv := map[string]string{}
	if fromStdin {
		if len(args) != 1 || strings.Contains(args[0], "=") {
			return nil, fmt.Errorf("--stdin requires exactly one bare KEY argument")
		}
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		v := string(b)
		v = strings.TrimSuffix(v, "\n")
		v = strings.TrimSuffix(v, "\r")
		kv[args[0]] = v
		return kv, nil
	}
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q (use --stdin for a bare KEY)", a)
		}
		kv[k] = v
	}
	return kv, nil
}

// stdinIsTerminal reports whether stdin is an interactive terminal.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
