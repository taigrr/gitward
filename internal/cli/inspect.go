package cli

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
	"github.com/taigrr/gitward/internal/merge"
)

// NewListCmd lists targets, envs, tiers, and key names (never values).
func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered targets, envs, tiers, and key names",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				c.Println("store not initialized (run 'ward init')")
				return nil
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
			for _, p := range paths {
				c.Println(p)
				envs := sp[p]
				envNames := make([]string, 0, len(envs))
				for en := range envs {
					envNames = append(envNames, en)
				}
				sort.Strings(envNames)
				for _, en := range envNames {
					for _, tier := range []string{"buildtime", "runtime"} {
						vals := envs[en][tierOf(tier)]
						if len(vals) == 0 {
							continue
						}
						keys := make([]string, 0, len(vals))
						for k := range vals {
							keys = append(keys, k)
						}
						sort.Strings(keys)
						c.Printf("  %s/%s: %v\n", en, tier, keys)
					}
				}
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewStatusCmd summarizes per-cell sync state.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync state of every registered leaf file vs the store",
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				c.Println("store not initialized (run 'ward init')")
				return nil
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			clean := true
			for _, cr := range plan {
				var incoming, local, conflict, del int
				for _, r := range cr.Results {
					switch r.Op {
					case merge.OpTakeStore:
						incoming++
					case merge.OpTakeLeaf:
						local++
					case merge.OpConflict:
						conflict++
					case merge.OpDelete:
						del++
					}
				}
				if incoming+local+conflict+del == 0 {
					continue
				}
				clean = false
				c.Printf("%s: incoming=%d local=%d delete=%d conflict=%d\n",
					cr.Cell, incoming, local, del, conflict)
			}
			if clean {
				c.Println("all in sync")
			}
			return nil
		},
	}
	addSSHFlag(cmd)
	return cmd
}

// NewDiffCmd shows per-key merge decisions.
func NewDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [path]",
		Short: "Show per-key differences between leaf files and the store",
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.Open(sshKeyFlag)
			if err != nil {
				return err
			}
			if !e.Initialized() {
				c.Println("store not initialized (run 'ward init')")
				return nil
			}
			plan, err := e.Plan()
			if err != nil {
				return err
			}
			filter := ""
			if len(args) == 1 {
				filter = args[0]
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
