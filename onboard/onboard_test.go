package onboard

import (
	"context"
	"os"

	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeAirdrop struct {
	requested []string
	balance   uint64
	fail      int
}

func (f *fakeAirdrop) RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error) {
	if f.fail > 0 {
		f.fail--
		return "", os.ErrDeadlineExceeded
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
	if res.Pubkey == "" || res.Balance != 1_000_000_000 {
		t.Errorf("result: %+v", res)
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

func TestInitRefusesExistingKey(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "key"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Init(InitOpts{
		Home:       home,
		ConfigPath: filepath.Join(home, "config"),
		Getenv:     func(string) string { return "" },
		RPC:        &fakeAirdrop{},
		Sleep:      noSleep,
	})
	if err == nil || !strings.Contains(err.Error(), "use --force") {
		t.Fatalf("want refusal on existing key, got %v", err)
	}
	res, err := Init(InitOpts{
		Home:            home,
		ConfigPath:      filepath.Join(home, "config"),
		Getenv:          func(string) string { return "" },
		RPC:             &fakeAirdrop{},
		AirdropLamports: 1_000_000_000,
		Sleep:           noSleep,
		Force:           true,
	})
	if err != nil {
		t.Fatal("force should overwrite:", err)
	}
	if res.Pubkey == "" {
		t.Fatal("no pubkey after force init")
	}
}

func TestInitAirdropRetries(t *testing.T) {
	home := t.TempDir()
	fake := &fakeAirdrop{fail: 2} // two failed requests, third succeeds
	res, err := Init(InitOpts{
		Home:            home,
		ConfigPath:      filepath.Join(home, "config"),
		Getenv:          func(string) string { return "" },
		RPC:             fake,
		AirdropLamports: 500_000_000,
		Sleep:           noSleep,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Balance != 500_000_000 {
		t.Errorf("balance %d", res.Balance)
	}
	if len(fake.requested) != 1 {
		t.Errorf("expected the third attempt to succeed, requested %d", len(fake.requested))
	}
}

func TestOperatorKeyPrecedence(t *testing.T) {
	kp := mustKeypair(t)
	envKp := mustKeypair(t)
	env := map[string]string{
		"ORBIT_OPERATOR_KEY":      mustSecret(envKp),
		"ORBIT_OPERATOR_KEY_PATH": "/nonexistent/env-path",
	}
	// Flag beats env.
	got, err := OperatorKey(func(k string) string { return env[k] }, mustSecret(kp), "")
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicBase58() != kpPublic(kp) {
		t.Errorf("flag should win over env, got %s", got.PublicBase58())
	}
	// Env path is used when no flag.
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
	// Missing key is loud.
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
