package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/sol"
)

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

func TestLoadOperatorCreateConfigNeedsNoCreator(t *testing.T) {
	env := map[string]string{
		"ORBIT_CONFIG":  filepath.Join(t.TempDir(), "config"),
		"ORBIT_INDEXER": "https://x",
		"ORBIT_RPC":     "https://x",
	}
	cfg, err := LoadOperatorCreateConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowWrite {
		t.Error("operator create config on devnet must keep the write gate on")
	}
	if cfg.VaultCreator != "" {
		t.Errorf("vault create must not require a prior creator, got %q", cfg.VaultCreator)
	}
	env["ORBIT_VAULT_CREATOR"] = "11111111111111111111111111111111"
	cfg, err = LoadOperatorCreateConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VaultCreator != "11111111111111111111111111111111" {
		t.Errorf("a present creator should still load: %q", cfg.VaultCreator)
	}
	delete(env, "ORBIT_INDEXER")
	if _, err := LoadOperatorCreateConfig(func(k string) string { return env[k] }); err == nil {
		t.Error("operator create config without the indexer accepted")
	}
}

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

func TestHomeDefaultsToOrbit(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HOME", scratch)
	t.Setenv("RIG_HOME", "")
	home, err := Home(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(scratch, ".orbit"); home != want {
		t.Errorf("home %s, want %s", home, want)
	}
}

func TestHomeHonorsRigHome(t *testing.T) {
	home, err := Home(func(k string) string {
		if k == "RIG_HOME" {
			return "/x/rig-home"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if home != "/x/rig-home" {
		t.Errorf("home %s, want /x/rig-home", home)
	}
}
