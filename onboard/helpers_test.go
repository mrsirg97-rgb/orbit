package onboard

import (
	"testing"

	"github.com/mrsirg97-rgb/orbit/sol"
)

func mustKeypair(t *testing.T) sol.Keypair {
	t.Helper()
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return kp
}

func kpPublic(kp sol.Keypair) string { return kp.PublicBase58() }

func mustSecret(kp sol.Keypair) string { return sol.Encode(kp.Secret) }
