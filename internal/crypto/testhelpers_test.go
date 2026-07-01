package crypto

import "filippo.io/age"

// testAgeKey is a generated native age keypair used by tests that need an age
// recipient without touching ~/.ssh.
type testAgeKey struct {
	recipient string
	id        *age.X25519Identity
}

func (k testAgeKey) identities() []age.Identity { return []age.Identity{k.id} }

func ageGenerate() (testAgeKey, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return testAgeKey{}, err
	}
	return testAgeKey{recipient: id.Recipient().String(), id: id}, nil
}
