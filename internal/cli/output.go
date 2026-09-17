package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/taigrr/gitward/internal/engine"
	"github.com/taigrr/gitward/internal/merge"
)

// Process exit codes. ExitFailure is the generic failure code (unchanged from
// earlier releases); ExitDrift and ExitConflict are only emitted by commands
// that document them (status --exit-code, sync).
const (
	ExitOK       = 0
	ExitFailure  = 1
	ExitDrift    = 1
	ExitConflict = 2
)

// jsonFlag is the global --json switch (machine-readable output).
var jsonFlag bool

// AddGlobalFlags registers persistent flags shared by every subcommand.
func AddGlobalFlags(root *cobra.Command) {
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "emit machine-readable JSON (data on stdout, errors on stderr)")
}

// ExitError carries a specific process exit code. When Silent is set the
// error handler prints nothing (the command already reported its result).
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode maps an error returned by cobra/fang to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitFailure
}

// errorKind names an error class for the JSON envelope so callers can branch
// without parsing the message.
func errorKind(err error) string {
	var ee *ExitError
	if errors.As(err, &ee) && ee.Code == ExitConflict {
		return "conflict"
	}
	return "error"
}

// ErrorHandler renders errors: a JSON envelope when --json is active, nothing
// for silent exit errors, and fang's default styling otherwise.
func ErrorHandler(w io.Writer, styles fang.Styles, err error) {
	var ee *ExitError
	if errors.As(err, &ee) && ee.Silent {
		return
	}
	if jsonFlag {
		writeJSON(os.Stderr, map[string]string{"error": err.Error(), "kind": errorKind(err)})
		return
	}
	fang.DefaultErrorHandler(w, styles, err)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printJSON writes v to the command's stdout as indented JSON.
func printJSON(c *cobra.Command, v any) error {
	return writeJSON(c.OutOrStdout(), v)
}

// errNotInitialized is the shared "run ward init" failure.
var errNotInitialized = errors.New("store not initialized (run 'ward init')")

// notInitialized reports the uninitialized-store condition. Read-only commands
// historically printed a note and exited 0; that is preserved for human output,
// while --json callers get a real error.
func notInitialized(c *cobra.Command) error {
	if jsonFlag {
		return errNotInitialized
	}
	c.Println(errNotInitialized.Error())
	return nil
}

// --- JSON shapes ---

// cellJSON is the machine-readable identity of a cell.
type cellJSON struct {
	Path string `json:"path"`
	Env  string `json:"env"`
	Tier string `json:"tier"`
	File string `json:"file"`
}

func toCellJSON(c engine.Cell) cellJSON {
	return cellJSON{Path: c.Path, Env: c.Env, Tier: string(c.Tier), File: c.File()}
}

// Op names used in JSON output and in `diff`'s human sigils.
const (
	opIncoming = "incoming"
	opLocal    = "local"
	opDelete   = "delete"
	opConflict = "conflict"
)

func opName(op merge.Op) string {
	switch op {
	case merge.OpTakeStore:
		return opIncoming
	case merge.OpTakeLeaf:
		return opLocal
	case merge.OpDelete:
		return opDelete
	case merge.OpConflict:
		return opConflict
	}
	return ""
}

// changeJSON is one pending per-key operation.
type changeJSON struct {
	cellJSON
	Key string `json:"key"`
	Op  string `json:"op"`
}

// planChanges flattens a plan into pending changes, optionally filtered to one
// cell path. Keys and values are never included.
func planChanges(plan []engine.CellResult, filter string) []changeJSON {
	out := []changeJSON{}
	for _, cr := range plan {
		if filter != "" && cr.Cell.Path != filter {
			continue
		}
		for _, r := range cr.Results {
			name := opName(r.Op)
			if name == "" {
				continue
			}
			out = append(out, changeJSON{cellJSON: toCellJSON(cr.Cell), Key: r.Key, Op: name})
		}
	}
	return out
}

// cellStatusJSON summarizes pending operations for one cell.
type cellStatusJSON struct {
	cellJSON
	Incoming int `json:"incoming"`
	Local    int `json:"local"`
	Delete   int `json:"delete"`
	Conflict int `json:"conflict"`
}

func (s cellStatusJSON) dirty() bool { return s.Incoming+s.Local+s.Delete+s.Conflict > 0 }

func summarize(cr engine.CellResult) cellStatusJSON {
	s := cellStatusJSON{cellJSON: toCellJSON(cr.Cell)}
	for _, r := range cr.Results {
		switch r.Op {
		case merge.OpTakeStore:
			s.Incoming++
		case merge.OpTakeLeaf:
			s.Local++
		case merge.OpConflict:
			s.Conflict++
		case merge.OpDelete:
			s.Delete++
		}
	}
	return s
}

// planConflicts counts conflicting keys across a plan.
func planConflicts(plan []engine.CellResult) int {
	n := 0
	for _, cr := range plan {
		for _, r := range cr.Results {
			if r.Conflict() {
				n++
			}
		}
	}
	return n
}
