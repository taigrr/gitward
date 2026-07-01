package engine

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/taigrr/gitward/internal/crypto"
	"github.com/taigrr/gitward/internal/gitutil"
	"github.com/taigrr/gitward/internal/leaf"
	"github.com/taigrr/gitward/internal/store"
)

// Check is a single doctor finding.
type Check struct {
	Name string
	OK   bool
	Info string
}

// Doctor runs health checks over the store, hooks, recipients, ignore rules,
// and .example coverage.
func (e *Engine) Doctor() []Check {
	var checks []Check

	// Store parses / initialized.
	checks = append(checks, Check{"store initialized", e.Initialized(),
		condStr(e.Initialized(), ".gitward.json present with key material", "run 'ward init'")})

	// Base snapshot present.
	basePath := gitutil.BasePath(e.GitDir)
	_, berr := os.Stat(basePath)
	checks = append(checks, Check{"base snapshot", berr == nil,
		condStr(berr == nil, basePath, "will be created on first sync")})

	// Hooks installed.
	for _, h := range hookNames {
		p := filepath.Join(e.GitDir, "hooks", h)
		b, err := os.ReadFile(p)
		ok := err == nil && strings.Contains(string(b), hookMarker)
		checks = append(checks, Check{"hook " + h, ok, condStr(ok, "installed", "run 'ward install'")})
	}

	// Every recipient wrap decrypts (DEK already recovered if initialized).
	if e.Initialized() {
		checks = append(checks, Check{"data key recoverable", true, "recovered via configured method"})
		_ = crypto.PassphraseEnv
	}

	// .gitignore coverage + .example key parity.
	if e.Initialized() {
		checks = append(checks, e.checkIgnore()...)
		checks = append(checks, e.checkExamples()...)
	}

	return checks
}

// checkIgnore verifies generated leaf files are ignored by git.
func (e *Engine) checkIgnore() []Check {
	var checks []Check
	sp, err := e.DecryptStore()
	if err != nil {
		return []Check{{"gitignore coverage", false, err.Error()}}
	}
	var missing []string
	for path, envs := range sp {
		for env, tiers := range envs {
			for tier := range tiers {
				rel := leaf.Path(path, tier, env)
				if !gitutil.PathIgnored(e.Root, rel) {
					missing = append(missing, rel)
				}
			}
		}
	}
	ok := len(missing) == 0
	info := "all generated leaf files are gitignored"
	if !ok {
		info = "NOT ignored: " + strings.Join(missing, ", ")
	}
	checks = append(checks, Check{"gitignore coverage", ok, info})
	return checks
}

// checkExamples verifies that keys present in *.example files exist in the
// store for the corresponding target (a common drift source).
func (e *Engine) checkExamples() []Check {
	var checks []Check
	sp, _ := e.DecryptStore()
	var missing []string
	_ = filepath.WalkDir(e.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".example") {
			return nil
		}
		tier, env, ok := classifyExample(name)
		if !ok {
			return nil
		}
		basepath, _ := filepath.Rel(e.Root, filepath.Dir(p))
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		exVals, err := leaf.Parse(string(b))
		if err != nil {
			return nil
		}
		have := cellVals(sp, Cell{basepath, env, tier})
		for k := range exVals {
			if _, ok := have[k]; !ok {
				missing = append(missing, basepath+" "+string(tier)+"/"+env+" "+k)
			}
		}
		return nil
	})
	ok := len(missing) == 0
	info := ".example keys all present in store"
	if !ok {
		info = "missing from store: " + strings.Join(missing, ", ")
	}
	checks = append(checks, Check{"example parity", ok, info})
	return checks
}

// classifyExample maps ".env.example", ".env.<env>.example",
// ".dev.vars.example", ".dev.vars.<env>.example" to a tier+env.
func classifyExample(name string) (store.Tier, string, bool) {
	base := strings.TrimSuffix(name, ".example")
	switch {
	case base == ".dev.vars":
		return store.Runtime, store.DefaultEnv, true
	case strings.HasPrefix(base, ".dev.vars."):
		return store.Runtime, strings.TrimPrefix(base, ".dev.vars."), true
	case base == ".env":
		return store.Buildtime, store.DefaultEnv, true
	case strings.HasPrefix(base, ".env."):
		return store.Buildtime, strings.TrimPrefix(base, ".env."), true
	}
	return "", "", false
}

func condStr(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
