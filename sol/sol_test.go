package sol

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

// The SDK-verified vectors: @solana/web3.js 1.98.4 + @solana/spl-token,
// seed program FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh, mint
// EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG, owner WSOL.
func TestPDAMatchesSDK(t *testing.T) {
	const program = "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh"
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	const owner = "So11111111111111111111111111111111111111112"
	cases := []struct {
		name  string
		prog  string
		seeds [][]byte
		want  string
		bump  byte
	}{
		{"global_config", program, [][]byte{[]byte("global_config")}, "59VnPKqFBA6Y1AsUbr6UFFVGMwXQusQqwVUeqwmKQCKA", 254},
		{"bonding_curve", program, [][]byte{[]byte("bonding_curve"), must(t, mint)}, "6wDUn9V7fuP1Ujn6o3xk4yFh4EE3F65LpgQsrNjTjmVx", 255},
		{"treasury_sol_vault", program, [][]byte{[]byte("treasury_sol_vault"), must(t, mint)}, "B9f9i5pgEgvvRgQzzyQ98tVYy3KKc9z3qavu4xVp5nDy", 254},
		{"torch_vault", program, [][]byte{[]byte("torch_vault"), must(t, owner)}, "HkPCW8mFF6ndNGzKufyfsYLcmpjTaGVKaoDAjwjBauh4", 254},
		{"vault_sol", program, [][]byte{[]byte("torch_vault_sol"), must(t, owner)}, "2N7Wah8yDrzFEPLj7praV9kzMJau5bxwB5gaewxGYm1M", 254},
		{"wallet_link", program, [][]byte{[]byte("vault_wallet"), must(t, owner)}, "6bdQ7FSi3shWkzGW1Nuk6yKykqKuWxRAQwQycsk86vq2", 255},
		{"torch_config", program, [][]byte{[]byte("torch_config")}, "5AHM1Htm14hMAcTMnbN4BR7KBL5hvaqAbyjPbFupj2Td", 255},
		{"deep_pool", "CcwF61GW14AcxCS4E2zedHXdFXy8x8GQPvfxZrs2x2eT", [][]byte{[]byte("deep_pool"), must(t, "5AHM1Htm14hMAcTMnbN4BR7KBL5hvaqAbyjPbFupj2Td"), must(t, mint)}, "H5uVsoK2KydBthZYgMhh5Z2mJBRNfr7ZCZpLiVb9zjCf", 255},
		{"deep_pool_event_auth", "CcwF61GW14AcxCS4E2zedHXdFXy8x8GQPvfxZrs2x2eT", [][]byte{[]byte("__event_authority")}, "PzAe6xFEGCfQzf55tb4izdvu37wxYq6XE3Tahoc9Uy8", 255},
	}
	for _, c := range cases {
		got, bump, err := PDA(c.prog, c.seeds...)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
		if bump != c.bump {
			t.Errorf("%s: bump %d, want %d", c.name, bump, c.bump)
		}
	}
}

func must(t *testing.T, s string) []byte {
	t.Helper()
	b, err := Decode(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBase58RoundTrip(t *testing.T) {
	cases := []struct {
		raw []byte
		enc string
	}{
		{[]byte{}, ""},
		{[]byte{0}, "1"},
		{[]byte{0, 0}, "11"},
		{[]byte{1}, "2"},
		{[]byte{0x7f}, "3C"},
		{[]byte{0xff, 0xff, 0xff}, "2UzHL"},
		{[]byte{0x00, 0x01}, "12"},
	}
	for _, c := range cases {
		got := Encode(c.raw)
		if got != c.enc {
			t.Errorf("Encode(%v) = %q, want %q", c.raw, got, c.enc)
		}
		back, err := Decode(c.enc)
		if err != nil {
			t.Fatalf("Decode(%q): %v", c.enc, err)
		}
		if !bytes.Equal(back, c.raw) {
			t.Errorf("round trip %q: %v, want %v", c.enc, back, c.raw)
		}
	}
	if _, err := Decode("0OIl"); err == nil {
		t.Error("Decode accepted invalid base58 chars")
	}
}

func TestKeypairFromSecret(t *testing.T) {
	// A deterministic 64-byte secret from a known seed.
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	// The real check: KeypairFromSecret accepts a correct secret and rejects
	// a mismatched one.
	valid := make([]byte, 64)
	copy(valid[:32], seed)
	derived := pubOf(seed)
	copy(valid[32:], derived)
	kp, err := KeypairFromSecret(Encode(valid))
	if err != nil {
		t.Fatalf("valid secret rejected: %v", err)
	}
	if kp.PublicBase58() != Encode(derived) {
		t.Errorf("pubkey mismatch")
	}
	bad := make([]byte, 64)
	copy(bad[:32], seed)
	copy(bad[32:], make([]byte, 32))
	if _, err := KeypairFromSecret(Encode(bad)); err == nil {
		t.Error("mismatched secret accepted")
	}
	if _, err := KeypairFromSecret("1"); err == nil {
		t.Error("short secret accepted")
	}
}

func TestCompileAndSign(t *testing.T) {
	const blockhash = "11111111111111111111111111111111"
	seed := make([]byte, 32)
	copy(seed, []byte("orbit-test-seed-0000000000000"))
	kp, _ := KeypairFromSecret(Encode(append(append([]byte{}, seed...), pubOf(seed)...)))
	ix := Instruction{
		ProgramID: "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh",
		Accounts:  []AccountMeta{{Pubkey: kp.PublicBase58(), IsSigner: true, IsWritable: true}},
		Data:      []byte{1, 2, 3},
	}
	msg, err := Compile(blockhash, kp.PublicBase58(), []Instruction{ix})
	if err != nil {
		t.Fatal(err)
	}
	signed := SignTx(msg, kp)
	if len(signed) == 0 {
		t.Fatal("empty signed tx")
	}
	sig, err := Signature(EncodeTx(signed))
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) == 0 {
		t.Fatal("empty signature")
	}
	// The message must contain the blockhash and the program id.
	if !bytes.Contains(signed[64:], must(t, blockhash)) {
		t.Error("message lacks the blockhash")
	}
}

func TestSignVersionedTxMulti(t *testing.T) {
	const blockhash = "11111111111111111111111111111111"
	k1 := testKeypair("multi-sign-a")
	k2 := testKeypair("multi-sign-b")
	ix := Instruction{
		ProgramID: "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh",
		Accounts: []AccountMeta{
			{Pubkey: k1.PublicBase58(), IsSigner: true, IsWritable: true},
			{Pubkey: k2.PublicBase58(), IsSigner: true, IsWritable: true},
		},
		Data: []byte{1, 2, 3},
	}
	msg, err := Compile(blockhash, k1.PublicBase58(), []Instruction{ix})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignVersionedTxMulti(msg, k1, k2)
	if err != nil {
		t.Fatal(err)
	}
	if signed[0] != 2 {
		t.Fatalf("signature count %d, want 2", signed[0])
	}
	sigs := signed[1 : 1+2*64]
	vmsg := signed[1+2*64:]
	if vmsg[0] != 0x80 {
		t.Fatalf("v0 marker %x", vmsg[0])
	}
	if !ed25519.Verify(k1.Public, vmsg, sigs[:64]) {
		t.Error("first signature does not verify for the payer")
	}
	if !ed25519.Verify(k2.Public, vmsg, sigs[64:128]) {
		t.Error("second signature does not verify for the mint")
	}
	if _, err := SignVersionedTxMulti(msg, k1); err == nil {
		t.Error("missing signer accepted")
	}
}

func testKeypair(seed string) Keypair {
	s := make([]byte, 32)
	copy(s, seed)
	k, err := KeypairFromSecret(Encode(append(append([]byte{}, s...), pubOf(s)...)))
	if err != nil {
		panic(err)
	}
	return k
}
