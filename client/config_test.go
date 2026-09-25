package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/sol"
)

// TestConfigPrecedence: the config file provides defaults, env overrides,
// and the key file path resolves the agent key.
func TestConfigPrecedence(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config")
	keyPath := filepath.Join(home, "key")
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(sol.Encode(kp.Secret)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := strings.Join([]string{
		"# defaults",
		"ORBIT_INDEXER=https://file.indexer",
		"ORBIT_RPC=https://file.rpc",
		"ORBIT_PROGRAM_ID=" + DevnetProgramID,
		"ORBIT_VAULT_CREATOR=11111111111111111111111111111111",
		"ORBIT_AGENT_KEY_FILE=" + keyPath,
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(file+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"ORBIT_CONFIG": cfgPath}
	cfg, err := LoadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Indexer != "https://file.indexer" || cfg.RPC != "https://file.rpc/rpc" {
		t.Errorf("file defaults (rpc should be host + /rpc): %+v", cfg)
	}
	if cfg.AgentKey.PublicBase58() != kp.PublicBase58() {
		t.Errorf("key file not resolved: %s", cfg.AgentKey.PublicBase58())
	}
	// Env overrides the file.
	env["ORBIT_INDEXER"] = "https://env.indexer"
	env["ORBIT_VAULT_CREATOR"] = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	cfg, err = LoadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Indexer != "https://env.indexer" {
		t.Errorf("env should override file: %s", cfg.Indexer)
	}
	if cfg.VaultCreator != "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy" {
		t.Errorf("vault creator from env: %s", cfg.VaultCreator)
	}
	// The inline env key beats the file path.
	env["ORBIT_AGENT_KEY"] = sol.Encode(kp.Secret)
	env["ORBIT_AGENT_KEY_FILE"] = "/nonexistent"
	cfg, err = LoadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentKey.PublicBase58() != kp.PublicBase58() {
		t.Errorf("inline key should win: %s", cfg.AgentKey.PublicBase58())
	}
}

// TestConfigRefusesNonDevnet: the gate stays — a non-devnet program refuses
// every load (the write gate is structural, not per-call).
func TestConfigRefusesNonDevnet(t *testing.T) {
	env := map[string]string{
		"ORBIT_INDEXER":       "https://x",
		"ORBIT_RPC":           "https://x",
		"ORBIT_PROGRAM_ID":    "So11111111111111111111111111111111111111112",
		"ORBIT_VAULT_CREATOR": "11111111111111111111111111111111",
		"ORBIT_AGENT_KEY":     sol.Encode(mustKeypairSecret(t)),
	}
	cfg, err := LoadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AllowWrite {
		t.Fatal("non-devnet config must have the write gate off")
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "devnet only") {
		t.Fatalf("non-devnet program must refuse writes, got %v", err)
	}
}

func mustKeypairSecret(t *testing.T) []byte {
	t.Helper()
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return kp.Secret
}

// TestDefaultRPCFromIndexer: ORBIT_RPC unset derives {indexer}/rpc; a set
// ORBIT_RPC (full endpoint or host-only) is normalized.
func TestDefaultRPCFromIndexer(t *testing.T) {
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]string{
		"ORBIT_INDEXER":       "https://api.torchmarket.dev",
		"ORBIT_VAULT_CREATOR": "11111111111111111111111111111111",
		"ORBIT_AGENT_KEY":     sol.Encode(kp.Secret),
	}
	cfg, err := LoadConfig(func(k string) string { return base[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RPC != "https://api.torchmarket.dev/rpc" {
		t.Errorf("derived RPC %q, want {indexer}/rpc", cfg.RPC)
	}

	// A set ORBIT_RPC wins, and a host-only value gets /rpc appended.
	env := map[string]string{
		"ORBIT_INDEXER":       "https://api.torchmarket.dev",
		"ORBIT_RPC":           "https://other.node",
		"ORBIT_VAULT_CREATOR": "11111111111111111111111111111111",
		"ORBIT_AGENT_KEY":     sol.Encode(kp.Secret),
	}
	cfg, err = LoadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RPC != "https://other.node/rpc" {
		t.Errorf("host-only RPC should be normalized: %q", cfg.RPC)
	}
	env["ORBIT_RPC"] = "https://api.torchmarket.dev/rpc"
	cfg, err = LoadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RPC != "https://api.torchmarket.dev/rpc" {
		t.Errorf("full endpoint should be used as-is: %q", cfg.RPC)
	}
}

// TestLoadReadConfigNeedsNoSigningKey: a read (project list) loads with only
// the indexer and rpc; a present-but-bad key still fails closed, and the
// write gate stays off.
func TestLoadReadConfigNeedsNoSigningKey(t *testing.T) {
	env := map[string]string{
		"ORBIT_CONFIG":  filepath.Join(t.TempDir(), "config"),
		"ORBIT_INDEXER": "https://x",
		"ORBIT_RPC":     "https://x",
	}
	cfg, err := LoadReadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AllowWrite {
		t.Error("read config must keep the write gate off")
	}
	if cfg.VaultCreator != "" {
		t.Errorf("vault creator must not be required for a read: %q", cfg.VaultCreator)
	}
	if _, err := LoadReadConfig(func(k string) string {
		return map[string]string{"ORBIT_CONFIG": filepath.Join(t.TempDir(), "config")}[k]
	}); err == nil {
		t.Error("read config without the indexer accepted")
	}
	bad := map[string]string{
		"ORBIT_INDEXER":   "https://x",
		"ORBIT_RPC":       "https://x",
		"ORBIT_AGENT_KEY": "1",
	}
	if _, err := LoadReadConfig(func(k string) string { return bad[k] }); err == nil {
		t.Error("read config with a bad agent key accepted")
	}
}

// TestLoadOperatorConfigNeedsNoAgentKey: the operator's write path (vault,
// project create) needs the public creator but not the agent's signing key.
func TestLoadOperatorConfigNeedsNoAgentKey(t *testing.T) {
	env := map[string]string{
		"ORBIT_CONFIG":        filepath.Join(t.TempDir(), "config"),
		"ORBIT_INDEXER":       "https://x",
		"ORBIT_RPC":           "https://x",
		"ORBIT_VAULT_CREATOR": "11111111111111111111111111111111",
	}
	cfg, err := LoadOperatorConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowWrite {
		t.Error("operator config on devnet must keep the write gate on")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("operator config must validate: %v", err)
	}
	delete(env, "ORBIT_VAULT_CREATOR")
	if _, err := LoadOperatorConfig(func(k string) string { return env[k] }); err == nil {
		t.Error("operator config without the vault creator accepted")
	}
}

// TestLoadBoardReadConfigNeedsOnlyRPC: a board read works with ORBIT_RPC
// set and nothing else — no indexer, no vault creator, no agent key.
func TestLoadBoardReadConfigNeedsOnlyRPC(t *testing.T) {
	env := map[string]string{
		"ORBIT_CONFIG": filepath.Join(t.TempDir(), "config"),
		"ORBIT_RPC":    "https://x",
	}
	cfg, err := LoadBoardReadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AllowWrite {
		t.Error("board read config must keep the write gate off")
	}
	if cfg.VaultCreator != "" || cfg.AgentKey.PublicBase58() != "" {
		t.Errorf("board read config must not carry a creator or key: %+v", cfg)
	}
	env["ORBIT_ENVFILE"] = filepath.Join(t.TempDir(), "none")
	noRPC := map[string]string{
		"ORBIT_CONFIG":  env["ORBIT_CONFIG"],
		"ORBIT_ENVFILE": env["ORBIT_ENVFILE"],
	}
	if _, err := LoadBoardReadConfig(func(k string) string { return noRPC[k] }); err == nil {
		t.Error("board read config without ORBIT_RPC accepted")
	}
	read, err := NewRead(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if read.RPC == nil || read.API == nil {
		t.Error("read client missing seams")
	}
	if read.Config.Indexer != "" || read.Config.VaultCreator != "" {
		t.Errorf("read client carries write state: %+v", read.Config)
	}
}
