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

// configFile is the orbit home's config path: KEY=VALUE defaults the env
// loader reads before env vars (env always wins).
func configFile(getenv func(string) string) (string, error) {
	if p := strings.TrimSpace(getenv("ORBIT_CONFIG")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("config: no ORBIT_CONFIG and no home directory")
	}
	return filepath.Join(home, ".config", "orbit", "config"), nil
}

// readConfigFile parses KEY=VALUE lines into a defaults map. A missing file
// is not an error (a bare env-only config is valid); an unreadable one is.
func readConfigFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		values[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return values, nil
}

// loadEnvFile populates the process environment from the legacy env file
// (only when the caller's env and the config file lack the required values).
func loadEnvFile(getenv func(string) string) error {
	path := strings.TrimSpace(getenv("ORBIT_ENVFILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("config: no ORBIT_ENVFILE and no home directory")
		}
		path = filepath.Join(home, ".config", "orbit", "env")
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
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

// LoadConfig reads the environment for the agent runtime. It fails closed:
// indexer, rpc, vault creator, and the agent key are required, an unparseable
// key or a program ID that is not the devnet IDL address is an error, and
// writes stay on only for the devnet program.
//
// LoadOperatorConfig is the operator's write path (vault/project): indexer,
// rpc, and the vault creator are required, the agent key is not. LoadReadConfig
// is the read-only path (project list): indexer and rpc only, writes stay off.
// Reads never need a signing key — a stranger browsing before deciding to join
// is the case.
//
// If the required ORBIT_* values are absent, the env file at $ORBIT_CONFIG
// (default ~/.config/orbit/env) is loaded first — the scheduled fire's cron
// environment carries no secrets, so the operator keeps them in that file
// (chmod 600; gitignored). The file is KEY=VALUE lines, '#' comments.
func LoadConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{agentKey: true, vaultCreator: true, writes: true})
}

// LoadOperatorConfig is the operator's write config: no agent key, the vault
// creator required, writes gated on the devnet program.
func LoadOperatorConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{vaultCreator: true, writes: true})
}

// LoadReadConfig is the read-only config: indexer + rpc only, writes off. An
// agent key or vault creator in the environment is still validated when
// present, never required.
func LoadReadConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{})
}

type requirements struct {
	agentKey     bool
	vaultCreator bool
	writes       bool
}

func loadConfig(getenv func(string) string, req requirements) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	path, err := configFile(getenv)
	if err != nil {
		return Config{}, err
	}
	defaults, err := readConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	// value(): env wins, then the config file, then the legacy env file.
	value := func(k string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return strings.TrimSpace(defaults[k])
	}
	if value("ORBIT_INDEXER") == "" || (req.agentKey && value("ORBIT_AGENT_KEY") == "" && value("ORBIT_AGENT_KEY_FILE") == "") {
		if err := loadEnvFile(getenv); err != nil {
			return Config{}, err
		}
	}
	indexer := value("ORBIT_INDEXER")
	rpc := value("ORBIT_RPC")
	if rpc == "" {
		rpc = strings.TrimSuffix(indexer, "/") + "/rpc"
	}
	if isHostOnly(rpc) {
		rpc = strings.TrimSuffix(rpc, "/") + "/rpc"
	}
	creator := value("ORBIT_VAULT_CREATOR")
	prog := value("ORBIT_PROGRAM_ID")
	if prog == "" {
		prog = DevnetProgramID
	}
	key := value("ORBIT_AGENT_KEY")
	if key == "" {
		keyPath := value("ORBIT_AGENT_KEY_FILE")
		if keyPath != "" {
			if strings.HasPrefix(keyPath, "~") {
				home, err := os.UserHomeDir()
				if err != nil {
					return Config{}, fmt.Errorf("config: ORBIT_AGENT_KEY_FILE: %w", err)
				}
				keyPath = filepath.Join(home, strings.TrimPrefix(keyPath, "~"))
			}
			b, err := os.ReadFile(keyPath)
			if err != nil {
				return Config{}, fmt.Errorf("config: ORBIT_AGENT_KEY_FILE %s: %w", keyPath, err)
			}
			key = strings.TrimSpace(string(b))
		}
	}

	var missing []string
	if indexer == "" {
		missing = append(missing, "ORBIT_INDEXER")
	}
	if rpc == "" {
		missing = append(missing, "ORBIT_RPC")
	}
	if req.vaultCreator && creator == "" {
		missing = append(missing, "ORBIT_VAULT_CREATOR")
	}
	if req.agentKey && key == "" {
		missing = append(missing, "ORBIT_AGENT_KEY")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config: missing %s (env, config file %s, or ORBIT_AGENT_KEY_FILE)", strings.Join(missing, ", "), path)
	}
	cfg := Config{
		Indexer:      indexer,
		RPC:          rpc,
		ProgramID:    prog,
		VaultCreator: creator,
	}
	if key != "" {
		kp, err := sol.KeypairFromSecret(key)
		if err != nil {
			return Config{}, fmt.Errorf("config: ORBIT_AGENT_KEY: %w", err)
		}
		cfg.AgentKey = kp
	}
	if req.writes && prog == DevnetProgramID {
		cfg.AllowWrite = true
	}
	return cfg, nil
}

// isHostOnly reports an endpoint with no path (the site base) — the JSON-RPC
// endpoint is {base}/rpc.
func isHostOnly(endpoint string) bool {
	slash := strings.Index(endpoint, "://")
	rest := endpoint
	if slash >= 0 {
		rest = endpoint[slash+3:]
	}
	return !strings.Contains(rest, "/")
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
