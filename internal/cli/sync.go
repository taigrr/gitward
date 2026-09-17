package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
)

// NewSyncCmd reconciles leaves and store in both directions.
func NewSyncCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile leaf files and store (both directions), advancing base",
		Long: "Apply every non-conflicting pending change: incoming store values are\n" +
			"written to leaf files and local leaf edits are captured into the store.\n" +
			"Exits 2 (without touching the store) when any conflict exists.\n\n" +
			"With --dry-run, prints what would change and applies nothing. With --json,\n" +
			"emits {applied, conflicts, changes:[{path, env, tier, file, key, op}]}.",
		RunE: func(c *cobra.Command, _ []string) error {
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
			changes := planChanges(plan, "")
			conflicts := planConflicts(plan)
			if dryRun {
				if jsonFlag {
					return printJSON(c, struct {
						Applied   bool         `json:"applied"`
						Conflicts int          `json:"conflicts"`
						Changes   []changeJSON `json:"changes"`
					}{false, conflicts, changes})
				}
				for _, ch := range changes {
					c.Printf("%s %s [%s/%s] %s\n", opSymbolByName(ch.Op), ch.Path, ch.Env, ch.Tier, ch.Key)
				}
				if conflicts > 0 {
					c.Printf("%d conflict(s) would block sync\n", conflicts)
				} else if len(changes) == 0 {
					c.Println("in sync (nothing to do)")
				}
				return nil
			}
			conflicts, err = e.Apply(plan, engine.Both)
			if err != nil {
				return err
			}
			if jsonFlag {
				if err := printJSON(c, struct {
					Applied   bool         `json:"applied"`
					Conflicts int          `json:"conflicts"`
					Changes   []changeJSON `json:"changes"`
				}{conflicts == 0, conflicts, changes}); err != nil {
					return err
				}
				if conflicts > 0 {
					return &ExitError{Code: ExitConflict, Silent: true}
				}
				return nil
			}
			if conflicts > 0 {
				return &ExitError{Code: ExitConflict, Err: fmt.Errorf("%d conflict(s) — run 'ward resolve'", conflicts)}
			}
			c.Println("in sync")
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show pending changes without applying them")
	addSSHFlag(cmd)
	return cmd
}

func opSymbolByName(name string) string {
	switch name {
	case opIncoming:
		return "<-"
	case opLocal:
		return "->"
	case opDelete:
		return "--"
	case opConflict:
		return "!!"
	}
	return "??"
}

// registerEntryJSON is one cell in `register --json`.
type registerEntryJSON struct {
	cellJSON
	Keys []string `json:"keys"`
}

// NewRegisterCmd scans for unregistered keys and adds them.
func NewRegisterCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "register [path]",
		Short: "Register new leaf files / unregistered keys into the store",
		Long: "Scan for .env / .dev.vars files (under path, or the whole repo) and add\n" +
			"any keys the store does not yet know about. Existing keys are left to sync.\n\n" +
			"With --dry-run, lists what would be registered without writing. With --json,\n" +
			"emits {registered, dry_run, cells:[{path, env, tier, file, keys}]}.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				return errNotInitialized
			}
			rel := ""
			if len(args) == 1 {
				rel, err = relToRoot(e.Root, args[0])
				if err != nil {
					return err
				}
			}
			entries, err := e.RegisterPlan(rel)
			if err != nil {
				return err
			}
			total := 0
			cells := make([]registerEntryJSON, 0, len(entries))
			for _, en := range entries {
				total += len(en.Keys)
				cells = append(cells, registerEntryJSON{cellJSON: toCellJSON(en.Cell), Keys: en.Keys})
			}
			if !dryRun && total > 0 {
				if _, err := e.Register(rel); err != nil {
					return err
				}
			}
			if jsonFlag {
				return printJSON(c, struct {
					Registered int                 `json:"registered"`
					DryRun     bool                `json:"dry_run"`
					Cells      []registerEntryJSON `json:"cells"`
				}{total, dryRun, cells})
			}
			if dryRun {
				for _, en := range entries {
					c.Printf("%s: %v\n", en.Cell, en.Keys)
				}
				c.Printf("would register %d new key(s)\n", total)
				return nil
			}
			c.Printf("registered %d new key(s)\n", total)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list unregistered keys without writing")
	addSSHFlag(cmd)
	return cmd
}
