package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mrsirg97-rgb/orbit/sol"
)

type Config struct {
	Indexer      string
	RPC          string
	ProgramID    string
	VaultCreator string
	AgentKey     sol.Keypair
	AllowWrite   bool
}

func Home(getenv func(string) string) (string, error) {
	if h := strings.TrimSpace(getenv("RIG_HOME")); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("config: no RIG_HOME and no home directory")
	}
	return filepath.Join(home, ".orbit"), nil
}

func configFile(getenv func(string) string) (string, error) {
	if p := strings.TrimSpace(getenv("ORBIT_CONFIG")); p != "" {
		return p, nil
	}
	h, err := Home(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "config"), nil
}

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

func loadEnvFile(getenv func(string) string) error {
	path := strings.TrimSpace(getenv("ORBIT_ENVFILE"))
	if path == "" {
		h, err := Home(getenv)
		if err != nil {
			return err
		}
		path = filepath.Join(h, "env")
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

func LoadConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{agentKey: true, vaultCreator: true, writes: true, indexer: true})
}

func LoadOperatorConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{vaultCreator: true, writes: true, indexer: true})
}

func LoadReadConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{indexer: true})
}

func LoadBoardConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{agentKey: true, vaultCreator: true, writes: true})
}

func LoadBoardReadConfig(getenv func(string) string) (Config, error) {
	return loadConfig(getenv, requirements{})
}

type requirements struct {
	agentKey     bool
	vaultCreator bool
	writes       bool
	indexer      bool
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
	if rpc == "" && indexer != "" {
		rpc = strings.TrimSuffix(indexer, "/") + "/rpc"
	}
	if rpc != "" && isHostOnly(rpc) {
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
	if req.indexer && indexer == "" {
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

func isHostOnly(endpoint string) bool {
	slash := strings.Index(endpoint, "://")
	rest := endpoint
	if slash >= 0 {
		rest = endpoint[slash+3:]
	}
	return !strings.Contains(rest, "/")
}

func (c Config) Validate() error {
	if c.RPC == "" || c.VaultCreator == "" {
		return errors.New("config: rpc and vault creator are required")
	}
	if c.ProgramID != DevnetProgramID {
		return fmt.Errorf("config: program %s is not the devnet program %s (devnet only)", c.ProgramID, DevnetProgramID)
	}
	if _, err := sol.Decode(c.VaultCreator); err != nil {
		return fmt.Errorf("config: vault creator: %w", err)
	}
	return nil
}
