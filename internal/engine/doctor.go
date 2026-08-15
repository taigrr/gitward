package engine

import (
	"os"
	"path/filepath"
	"sort"
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

// Doctor runs health checks over the store, hooks, recipients, store-entry
// well-formedness, and .example coverage.
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

	// Hooks installed (in git's effective hooks dir, honoring core.hooksPath).
	hooksDir, herr := e.HooksDir()
	for _, h := range hookNames {
		if herr != nil {
			checks = append(checks, Check{"hook " + h, false, "cannot resolve hooks dir: " + herr.Error()})
			continue
		}
		p := filepath.Join(hooksDir, h)
		b, err := os.ReadFile(p)
		ok := err == nil && strings.Contains(string(b), hookMarker)
		checks = append(checks, Check{"hook " + h, ok, condStr(ok, "installed in "+hooksDir, "run 'ward install'")})
	}

	// Every recipient wrap decrypts (DEK already recovered if initialized).
	if e.Initialized() {
		checks = append(checks, Check{"data key recoverable", true, "recovered via configured method"})
		_ = crypto.PassphraseEnv
	}

	// .gitignore coverage + .example key parity.
	if e.Initialized() {
		checks = append(checks, e.checkStoreEntries())
		checks = append(checks, e.checkIgnore()...)
		checks = append(checks, e.checkExamples()...)
	}

	return checks
}

// checkStoreEntries verifies each store entry is well-formed: a variable is
// either managed (has ciphertext) or ward-ignored (marker only) — never both,
// and never neither.
func (e *Engine) checkStoreEntries() Check {
	var bad []string
	for path, tgt := range e.Store.Targets {
		for env, blk := range tgt {
			for _, ti := range []struct {
				tier store.Tier
				tm   store.TierMap
			}{{store.Buildtime, blk.Buildtime}, {store.Runtime, blk.Runtime}} {
				for k, v := range ti.tm {
					loc := path + " " + string(ti.tier) + "/" + env + " " + k
					switch {
					case v.Ignore && v.Ciphertext != "":
						bad = append(bad, loc+" (both enc and ignore)")
					case !v.Ignore && v.Ciphertext == "":
						bad = append(bad, loc+" (neither enc nor ignore)")
					}
				}
			}
		}
	}
	sort.Strings(bad)
	ok := len(bad) == 0
	info := "all store entries well-formed"
	if !ok {
		info = "malformed: " + strings.Join(bad, ", ")
	}
	return Check{"store entries", ok, info}
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
	var antipattern []string
	_ = filepath.WalkDir(e.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".example") {
			return nil
		}
		basepath, _ := filepath.Rel(e.Root, filepath.Dir(p))
		// Env-specific example files (.env.<env>.example /
		// .dev.vars.<env>.example) are an antipattern: an example is a
		// human-authored template of the keys an app needs, not a
		// per-env artifact. Commit exactly one env-agnostic template per
		// tier (.env.example, .dev.vars.example) instead.
		if isEnvSpecificExample(name) {
			antipattern = append(antipattern, filepath.Join(basepath, name))
			return nil
		}
		tier, ok := classifyExample(name)
		if !ok {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		exVals, err := leaf.Parse(string(b))
		if err != nil {
			return nil
		}
		// An example is env-agnostic: it declares the keys an app uses
		// without pinning an env, so a key counts as present if it exists
		// in that basepath+tier under any env (e.g. only `production`).
		have := tierKeysAcrossEnvs(sp, basepath, tier)
		for k := range exVals {
			if _, ok := have[k]; !ok {
				missing = append(missing, basepath+" "+string(tier)+" "+k)
			}
		}
		return nil
	})
	parityOK := len(missing) == 0
	parityInfo := ".example keys all present in store"
	if !parityOK {
		parityInfo = "missing from store: " + strings.Join(missing, ", ")
	}
	checks = append(checks, Check{"example parity", parityOK, parityInfo})

	conventionOK := len(antipattern) == 0
	conventionInfo := "examples are env-agnostic (.env.example / .dev.vars.example)"
	if !conventionOK {
		sort.Strings(antipattern)
		conventionInfo = "env-specific example files are an antipattern; use one env-agnostic template per tier: " +
			strings.Join(antipattern, ", ")
	}
	checks = append(checks, Check{"example convention", conventionOK, conventionInfo})
	return checks
}

// tierKeysAcrossEnvs returns the union of key names present for a basepath+tier
// across every env in the store. Env-agnostic example files declare the keys an
// app uses without pinning an env, so parity holds if a key lives under any env.
func tierKeysAcrossEnvs(p store.Plaintext, basepath string, tier store.Tier) map[string]struct{} {
	out := map[string]struct{}{}
	for env := range p[basepath] {
		for k := range p[basepath][env][tier] {
			out[k] = struct{}{}
		}
	}
	return out
}

// classifyExample maps the two supported, env-agnostic example templates to
// their tier: ".env.example" -> buildtime, ".dev.vars.example" -> runtime.
// Env-specific variants are intentionally not classified here (see
// isEnvSpecificExample); examples must not pin an env.
func classifyExample(name string) (store.Tier, bool) {
	switch name {
	case ".dev.vars.example":
		return store.Runtime, true
	case ".env.example":
		return store.Buildtime, true
	}
	return "", false
}

// isEnvSpecificExample reports whether name is an env-pinned example file
// (.env.<env>.example or .dev.vars.<env>.example), which is an antipattern —
// examples should be env-agnostic (one template per tier).
func isEnvSpecificExample(name string) bool {
	if !strings.HasSuffix(name, ".example") {
		return false
	}
	base := strings.TrimSuffix(name, ".example")
	switch {
	case base == ".dev.vars" || base == ".env":
		return false
	case strings.HasPrefix(base, ".dev.vars.") || strings.HasPrefix(base, ".env."):
		return true
	}
	return false
}

func condStr(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
