package crypto

// SetScryptWorkFactorForTesting lowers the passphrase KDF cost in tests that
// create many throwaway stores. It returns a restore function for callers that
// need to preserve the previous setting.
func SetScryptWorkFactorForTesting(logN int) func() {
	old := scryptWorkFactor
	scryptWorkFactor = logN
	return func() {
		scryptWorkFactor = old
	}
}
