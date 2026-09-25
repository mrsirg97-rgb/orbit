package onboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/sol"
)

type fakeAirdrop struct {
	requested []string
	balance   uint64
	fail      int
	err       error
}

func (f *fakeAirdrop) RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error) {
	if f.fail > 0 {
		f.fail--
		if f.err != nil {
			return "", f.err
		}
		return "", errors.New("rate limited")
	}
	f.requested = append(f.requested, pubkey)
	f.balance = lamports
	return "fake-sig", nil
}

func (f *fakeAirdrop) GetBalance(ctx context.Context, pubkey string) (uint64, error) {
	return f.balance, nil
}

func noSleep(time.Duration) {}

func TestInitOnEmptyHome(t *testing.T) {
	home := t.TempDir()
	fake := &fakeAirdrop{}
	res, err := Init(InitOpts{
		Home:            home,
		ConfigPath:      filepath.Join(home, "config"),
		Getenv:          func(string) string { return "" },
		RPC:             fake,
		AirdropLamports: 1_000_000_000,
		Sleep:           noSleep,
		AirdropBudget:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, "key")
	st, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal("key missing:", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("key perms %o, want 600", st.Mode().Perm())
	}
	b, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimSpace(string(b))
	if len(secret) < 80 {
		t.Errorf("key file is not a base58 secret: %q", secret)
	}
	cfg, err := os.ReadFile(res.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ORBIT_INDEXER=", "ORBIT_RPC=", "ORBIT_PROGRAM_ID=", "ORBIT_AGENT_KEY_FILE=" + keyPath} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("config lacks %s:\n%s", want, cfg)
		}
	}
	if res.Pubkey == "" || res.Balance != 1_000_000_000 || !res.AirdropOK {
		t.Errorf("result: %+v", res)
	}
	if res.ReusedKey {
		t.Error("fresh init must not reuse a key")
	}
	if len(fake.requested) != 1 {
		t.Errorf("airdrop requested %d times, want 1", len(fake.requested))
	}
	found := false
	for _, line := range res.Next {
		if strings.Contains(line, res.Pubkey) {
			found = true
		}
	}
	if !found {
		t.Errorf("next commands do not name the hot wallet: %v", res.Next)
	}
}

func TestInitTwiceReusesKey(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config")
	fake := &fakeAirdrop{}
	opts := InitOpts{
		Home: home, ConfigPath: cfgPath, Getenv: func(string) string { return "" },
		RPC: fake, AirdropLamports: 1_000_000_000, Sleep: noSleep, AirdropBudget: 0,
	}
	first, err := Init(opts)
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteConfigValue(cfgPath, "ORBIT_VAULT_CREATOR", "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"); err != nil {
		t.Fatal(err)
	}
	second, err := Init(opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Pubkey != first.Pubkey {
		t.Errorf("second init changed the key: %s -> %s", first.Pubkey, second.Pubkey)
	}
	if !second.ReusedKey {
		t.Error("second init must reuse the existing key")
	}

	if len(fake.requested) != 2 {
		t.Errorf("airdrop runs every init: requested %d, want 2", len(fake.requested))
	}
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "ORBIT_VAULT_CREATOR=8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy") {
		t.Errorf("operator edit lost:\n%s", cfg)
	}
}

func TestInitForceRegeneratesKey(t *testing.T) {
	home := t.TempDir()
	fake := &fakeAirdrop{}
	opts := InitOpts{
		Home: home, ConfigPath: filepath.Join(home, "config"), Getenv: func(string) string { return "" },
		RPC: fake, AirdropLamports: 1_000_000_000, Sleep: noSleep, AirdropBudget: 0,
	}
	first, err := Init(opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.Force = true
	second, err := Init(opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Pubkey == first.Pubkey {
		t.Error("--force must regenerate the key")
	}
}

func TestInitOwnHome(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HOME", scratch)
	t.Setenv("RIG_HOME", "")
	fake := &fakeAirdrop{}
	res, err := Init(InitOpts{
		Getenv:          os.Getenv,
		RPC:             fake,
		AirdropLamports: 1_000_000_000,
		Sleep:           noSleep,
		AirdropBudget:   0,
	})
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(scratch, ".orbit")
	for _, p := range []string{filepath.Join(home, "key"), filepath.Join(home, "config")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing: %v", p, err)
		}
	}
	if res.KeyPath != filepath.Join(home, "key") || res.ConfigPath != filepath.Join(home, "config") {
		t.Errorf("paths: key %s config %s, want %s / %s", res.KeyPath, res.ConfigPath, filepath.Join(home, "key"), filepath.Join(home, "config"))
	}
	if _, err := os.Stat(filepath.Join(scratch, ".rig")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("~/.rig must not exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(scratch, ".config", "orbit")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old ~/.config/orbit home must not exist: %v", err)
	}
}

func TestInitAirdropRetries(t *testing.T) {
	home := t.TempDir()
	fake := &fakeAirdrop{fail: 2}
	res, err := Init(InitOpts{
		Home: home, ConfigPath: filepath.Join(home, "config"), Getenv: func(string) string { return "" },
		RPC: fake, AirdropLamports: 500_000_000, Sleep: noSleep, AirdropBudget: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Balance != 500_000_000 || !res.AirdropOK {
		t.Errorf("retries should fund: %+v", res)
	}
	if len(fake.requested) != 1 {
		t.Errorf("expected the third attempt to succeed, requested %d", len(fake.requested))
	}
}

func TestInitAirdrop405And429(t *testing.T) {
	for _, status := range []error{
		errors.New("status 405: Bad method"),
		errors.New("status 429: Too many airdrop requests"),
	} {
		home := t.TempDir()
		fake := &fakeAirdrop{fail: 100, err: status, balance: 12345}
		res, err := Init(InitOpts{
			Home: home, ConfigPath: filepath.Join(home, "config"), Getenv: func(string) string { return "" },
			RPC: fake, AirdropLamports: 1_000_000_000, Sleep: noSleep, AirdropBudget: 0,
		})
		if err != nil {
			t.Fatalf("%v: init must succeed anyway: %v", status, err)
		}
		if res.AirdropOK {
			t.Errorf("%v: should be unfunded", status)
		}
		if res.Balance != 12345 {
			t.Errorf("%v: balance %d, want the current balance", status, res.Balance)
		}
		if !strings.Contains(res.FundingHint, res.Pubkey) || !strings.Contains(res.FundingHint, "faucet.solana.com") {
			t.Errorf("%v: hint %q", status, res.FundingHint)
		}
		if res.ReusedKey || res.Pubkey == "" {
			t.Errorf("%v: key should be usable: %+v", status, res)
		}
	}
}

func TestOperatorKeyPrecedence(t *testing.T) {
	kp := mustKeypair(t)
	envKp := mustKeypair(t)
	env := map[string]string{
		"ORBIT_OPERATOR_KEY":      mustSecret(envKp),
		"ORBIT_OPERATOR_KEY_PATH": "/nonexistent/env-path",
	}
	got, err := OperatorKey(func(k string) string { return env[k] }, mustSecret(kp), "")
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicBase58() != kpPublic(kp) {
		t.Errorf("flag should win over env, got %s", got.PublicBase58())
	}
	keyFile := filepath.Join(t.TempDir(), "operator.json")
	if err := os.WriteFile(keyFile, []byte(mustSecret(envKp)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env["ORBIT_OPERATOR_KEY"] = ""
	env["ORBIT_OPERATOR_KEY_PATH"] = keyFile
	got, err = OperatorKey(func(k string) string { return env[k] }, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicBase58() != kpPublic(envKp) {
		t.Errorf("env path should resolve, got %s", got.PublicBase58())
	}
	if _, err := OperatorKey(func(string) string { return "" }, "", ""); err == nil {
		t.Fatal("missing operator key should error")
	}
}

func TestWriteConfigValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := WriteConfigValue(path, "ORBIT_INDEXER", "https://a"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(path, "ORBIT_INDEXER", "https://b"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(path, "ORBIT_RPC", "https://rpc"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	if !strings.Contains(text, "ORBIT_INDEXER=https://b") || strings.Contains(text, "ORBIT_INDEXER=https://a") {
		t.Errorf("upsert failed:\n%s", text)
	}
	if !strings.Contains(text, "ORBIT_RPC=https://rpc") {
		t.Errorf("append failed:\n%s", text)
	}
}

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
