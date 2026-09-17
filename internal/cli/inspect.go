package cli

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
	"github.com/taigrr/gitward/internal/merge"
)

// listCellJSON is one cell in `list --json`.
type listCellJSON struct {
	cellJSON
	Keys    []string `json:"keys"`
	Ignored []string `json:"ignored"`
}

// NewListCmd lists targets, envs, tiers, and key names (never values).
func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered targets, envs, tiers, and key names",
		Long: "List every registered cell with its key names. Values are never printed.\n\n" +
			"With --json, emits an array of {path, env, tier, file, keys, ignored}.",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return notInitialized(c)
			}
			sp, err := e.DecryptStore()
			if err != nil {
				return err
			}
			paths := make([]string, 0, len(sp))
			for p := range sp {
				paths = append(paths, p)
			}
			sort.Strings(paths)
			var cells []listCellJSON
			for _, p := range paths {
				if !jsonFlag {
					c.Println(p)
				}
				envs := sp[p]
				envNames := make([]string, 0, len(envs))
				for en := range envs {
					envNames = append(envNames, en)
				}
				sort.Strings(envNames)
				for _, en := range envNames {
					for _, tier := range []string{"buildtime", "runtime"} {
						cell := engine.Cell{Path: p, Env: en, Tier: tierOf(tier)}
						vals := envs[en][cell.Tier]
						ignored := e.IgnoredKeys(cell)
						if len(vals) == 0 && len(ignored) == 0 {
							continue
						}
						keys := make([]string, 0, len(vals))
						for k := range vals {
							keys = append(keys, k)
						}
						sort.Strings(keys)
						if jsonFlag {
							if ignored == nil {
								ignored = []string{}
							}
							sort.Strings(ignored)
							cells = append(cells, listCellJSON{cellJSON: toCellJSON(cell), Keys: keys, Ignored: ignored})
							continue
						}
						if len(keys) == 0 {
							continue
						}
						c.Printf("  %s/%s: %v\n", en, tier, keys)
					}
				}
			}
			if jsonFlag {
				if cells == nil {
					cells = []listCellJSON{}
				}
				return printJSON(c, cells)
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// statusJSON is the payload of `status --json`.
type statusJSON struct {
	Clean     bool             `json:"clean"`
	Conflicts int              `json:"conflicts"`
	Cells     []cellStatusJSON `json:"cells"`
}

// NewStatusCmd summarizes per-cell sync state.
func NewStatusCmd() *cobra.Command {
	var exitCode bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync state of every registered leaf file vs the store",
		Long: "Summarize pending operations per cell.\n\n" +
			"With --exit-code the process exits 0 when everything is in sync, 1 when\n" +
			"there are pending (non-conflicting) changes, and 2 when any conflict exists.\n" +
			"With --json, emits {clean, conflicts, cells:[{path, env, tier, file,\n" +
			"incoming, local, delete, conflict}]} (only dirty cells are listed).",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return notInitialized(c)
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			out := statusJSON{Clean: true, Cells: []cellStatusJSON{}}
			for _, cr := range plan {
				s := summarize(cr)
				if !s.dirty() {
					continue
				}
				out.Clean = false
				out.Conflicts += s.Conflict
				out.Cells = append(out.Cells, s)
				if !jsonFlag {
					c.Printf("%s: incoming=%d local=%d delete=%d conflict=%d\n",
						cr.Cell, s.Incoming, s.Local, s.Delete, s.Conflict)
				}
			}
			if jsonFlag {
				if err := printJSON(c, out); err != nil {
					return err
				}
			} else if out.Clean {
				c.Println("all in sync")
			}
			if exitCode {
				switch {
				case out.Conflicts > 0:
					return &ExitError{Code: ExitConflict, Silent: true}
				case !out.Clean:
					return &ExitError{Code: ExitDrift, Silent: true}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit 1 when changes are pending, 2 when conflicts exist")
	addSSHFlag(cmd)
	return cmd
}

// NewDiffCmd shows per-key merge decisions.
func NewDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [path]",
		Short: "Show per-key differences between leaf files and the store",
		Long: "Print one line per pending key operation: '<-' incoming (store -> leaf),\n" +
			"'->' local edit (leaf -> store), '--' delete, '!!' conflict. Values are\n" +
			"never printed. An optional path restricts output to one target directory.\n\n" +
			"With --json, emits an array of {path, env, tier, file, key, op} where op is\n" +
			"one of incoming, local, delete, conflict.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return notInitialized(c)
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			filter, err := pathFilter(e.Root, args)
			if err != nil {
				return err
			}
			changes := planChanges(plan, filter)
			if jsonFlag {
				return printJSON(c, changes)
			}
			for _, cr := range plan {
				if filter != "" && cr.Cell.Path != filter {
					continue
				}
				for _, r := range cr.Results {
					sym := opSymbol(r.Op)
					if sym == "" {
						continue
					}
					c.Printf("%s %s %s\n", sym, cr.Cell, r.Key)
				}
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// pathFilter resolves the optional [path] argument of plan-scoped commands to
// a repo-relative target path. It accepts a directory or a leaf file path; a
// missing argument (or the repo root itself) means "everything".
func pathFilter(root string, args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	arg := args[0]
	if engine.IsLeafFileName(arg) {
		rel, err := relToRoot(root, arg)
		if err != nil {
			return "", err
		}
		cell, err := engine.ParseCellFile(rel)
		if err != nil {
			return "", err
		}
		return cell.Path, nil
	}
	return relToRoot(root, arg)
}

func opSymbol(op merge.Op) string {
	switch op {
	case merge.OpTakeStore:
		return "<-" // store -> leaf (incoming)
	case merge.OpTakeLeaf:
		return "->" // leaf -> store (local edit)
	case merge.OpDelete:
		return "--"
	case merge.OpConflict:
		return "!!"
	}
	return ""
}
