package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mrsirg97-rgb/orbit/sol"
)

// Config is the env-only client configuration. The agent hot key is the only
// secret the process ever holds; the operator's vault authority key never
// enters here.
type Config struct {
	Indexer      string
	RPC          string
	ProgramID    string
	VaultCreator string
	AgentKey     sol.Keypair
	AllowWrite   bool
}

// loadEnvFile populates the process environment from the operator's env file
// (only when the caller's env lacks the required values).
func loadEnvFile(getenv func(string) string) error {
	path := strings.TrimSpace(getenv("ORBIT_CONFIG"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("config: no ORBIT_CONFIG and no home directory")
		}
		path = filepath.Join(home, ".config", "orbit", "env")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: env file %s: %w", path, err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return nil
}

// LoadConfig reads the environment. It fails closed: any missing required
// value, an unparseable key, or a program ID that is not the devnet IDL
// address is an error, and writes stay off unless explicitly devnet.
//
// If the required ORBIT_* values are absent, the env file at $ORBIT_CONFIG
// (default ~/.config/orbit/env) is loaded first — the scheduled fire's cron
// environment carries no secrets, so the operator keeps them in that file
// (chmod 600; gitignored). The file is KEY=VALUE lines, '#' comments.
func LoadConfig(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if strings.TrimSpace(getenv("ORBIT_INDEXER")) == "" ||
		strings.TrimSpace(getenv("ORBIT_AGENT_KEY")) == "" {
		if err := loadEnvFile(getenv); err != nil {
			return Config{}, err
		}
	}
	indexer := strings.TrimSpace(getenv("ORBIT_INDEXER"))
	rpc := strings.TrimSpace(getenv("ORBIT_RPC"))
	creator := strings.TrimSpace(getenv("ORBIT_VAULT_CREATOR"))
	prog := strings.TrimSpace(getenv("ORBIT_PROGRAM_ID"))
	if prog == "" {
		prog = DevnetProgramID
	}
	key := strings.TrimSpace(getenv("ORBIT_AGENT_KEY"))

	var missing []string
	if indexer == "" {
		missing = append(missing, "ORBIT_INDEXER")
	}
	if rpc == "" {
		missing = append(missing, "ORBIT_RPC")
	}
	if creator == "" {
		missing = append(missing, "ORBIT_VAULT_CREATOR")
	}
	if key == "" {
		missing = append(missing, "ORBIT_AGENT_KEY")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config: missing env %s", strings.Join(missing, ", "))
	}
	kp, err := sol.KeypairFromSecret(key)
	if err != nil {
		return Config{}, fmt.Errorf("config: ORBIT_AGENT_KEY: %w", err)
	}
	cfg := Config{
		Indexer:      indexer,
		RPC:          rpc,
		ProgramID:    prog,
		VaultCreator: creator,
		AgentKey:     kp,
	}
	if prog == DevnetProgramID {
		cfg.AllowWrite = true
	}
	return cfg, nil
}

// Validate refuses a config that would write to a non-devnet cluster.
func (c Config) Validate() error {
	if c.Indexer == "" || c.RPC == "" || c.VaultCreator == "" {
		return errors.New("config: indexer, rpc, and vault creator are required")
	}
	if c.ProgramID != DevnetProgramID {
		return fmt.Errorf("config: program %s is not the devnet program %s (devnet only)", c.ProgramID, DevnetProgramID)
	}
	if _, err := sol.Decode(c.VaultCreator); err != nil {
		return fmt.Errorf("config: vault creator: %w", err)
	}
	return nil
}
