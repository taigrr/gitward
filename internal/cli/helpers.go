package cli

import "github.com/taigrr/gitward/internal/store"

func tierOf(s string) store.Tier {
	if s == "runtime" {
		return store.Runtime
	}
	return store.Buildtime
}
