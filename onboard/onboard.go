// Package onboard is the one-minute onboarding path: generate the agent hot
// wallet, write the orbit home config, fund it on devnet, and print the next
// commands. The operator key never passes through here — it is resolved per
// vault call (flag > env) and used only in the process.
package onboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/sol"
)

// Home is the orbit home (ORBIT_HOME, default ~/.config/orbit).
func Home(getenv func(string) string) (string, error) {
	if h := strings.TrimSpace(getenv("ORBIT_HOME")); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("onboard: no ORBIT_HOME and no home directory")
	}
	return filepath.Join(home, ".config", "orbit"), nil
}

// ConfigPath is the config file the env loader reads as defaults (ORBIT_CONFIG
// or the orbit home's config).
func ConfigPath(getenv func(string) string) (string, error) {
	if p := strings.TrimSpace(getenv("ORBIT_CONFIG")); p != "" {
		return p, nil
	}
	h, err := Home(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "config"), nil
}

// Airdrop is the devnet faucet seam (client.RPC in production).
type Airdrop interface {
	RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error)
	GetBalance(ctx context.Context, pubkey string) (uint64, error)
}

// InitOpts are the injectable inputs (tests swap home/getenv/rpc/sleep).
type InitOpts struct {
	Home            string
	ConfigPath      string
	Getenv          func(string) string
	RPC             Airdrop
	AirdropLamports uint64
	Sleep           func(time.Duration)
	Force           bool
}

// InitResult is what init prints.
type InitResult struct {
	Pubkey     string
	Balance    uint64
	KeyPath    string
	ConfigPath string
	Next       []string
}

// Init generates the hot wallet (0600), writes the config defaults, funds it
// on devnet (with retries), and returns the summary. It refuses to overwrite
// an existing key without Force.
func Init(opts InitOpts) (InitResult, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	home := opts.Home
	if home == "" {
		var err error
		home, err = Home(getenv)
		if err != nil {
			return InitResult{}, err
		}
	}
	cfgPath := opts.ConfigPath
	if cfgPath == "" {
		var err error
		cfgPath, err = ConfigPath(getenv)
		if err != nil {
			return InitResult{}, err
		}
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return InitResult{}, fmt.Errorf("init: %w", err)
	}
	keyPath := filepath.Join(home, "key")
	if _, err := os.Stat(keyPath); err == nil && !opts.Force {
		return InitResult{}, fmt.Errorf("init: key %s exists (use --force to overwrite)", keyPath)
	}

	kp, err := sol.GenerateKeypair()
	if err != nil {
		return InitResult{}, err
	}
	secret := sol.Encode(kp.Secret)
	if err := os.WriteFile(keyPath, []byte(secret+"\n"), 0o600); err != nil {
		return InitResult{}, fmt.Errorf("init: write key: %w", err)
	}

	value := func(k, fallback string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return fallback
	}
	lines := []string{
		"# orbit agent config: defaults the env loader reads (env always overrides)",
		"# written by 'orbit init' on " + time.Now().UTC().Format(time.RFC3339),
		"ORBIT_INDEXER=" + value("ORBIT_INDEXER", client.DefaultIndexer),
		"ORBIT_RPC=" + value("ORBIT_RPC", client.DefaultRPC),
		"ORBIT_PROGRAM_ID=" + value("ORBIT_PROGRAM_ID", client.DevnetProgramID),
	}
	if creator := strings.TrimSpace(getenv("ORBIT_VAULT_CREATOR")); creator != "" {
		lines = append(lines, "ORBIT_VAULT_CREATOR="+creator)
	}
	lines = append(lines, "ORBIT_AGENT_KEY_FILE="+keyPath)
	if err := os.WriteFile(cfgPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return InitResult{}, fmt.Errorf("init: write config: %w", err)
	}

	lamports := opts.AirdropLamports
	if lamports == 0 {
		lamports = 1_000_000_000 // 1 SOL
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	balance, err := airdropWithRetries(context.Background(), opts.RPC, kp.PublicBase58(), lamports, sleep)
	if err != nil {
		return InitResult{}, err
	}
	pub := kp.PublicBase58()
	return InitResult{
		Pubkey:     pub,
		Balance:    balance,
		KeyPath:    keyPath,
		ConfigPath: cfgPath,
		Next: []string{
			"export ORBIT_OPERATOR_KEY_PATH=/path/to/your/operator.json",
			"orbit vault create     # the vault is created for your operator key",
			"orbit vault link " + pub,
			"orbit vault deposit 1",
			"orbit agent register --model <fleet-model>",
		},
	}, nil
}

// airdropWithRetries requests the airdrop and polls the balance; on a failed
// request or a zero balance after the poll window it retries (up to 3).
func airdropWithRetries(ctx context.Context, rpc Airdrop, pubkey string, lamports uint64, sleep func(time.Duration)) (uint64, error) {
	if rpc == nil {
		return 0, errors.New("init: no RPC seam (devnet airdrop)")
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := rpc.RequestAirdrop(ctx, pubkey, lamports); err != nil {
			lastErr = fmt.Errorf("airdrop request: %w", err)
			sleep(2 * time.Second)
			continue
		}
		for poll := 0; poll < 4; poll++ {
			balance, err := rpc.GetBalance(ctx, pubkey)
			if err != nil {
				lastErr = fmt.Errorf("airdrop balance: %w", err)
				sleep(time.Second)
				continue
			}
			if balance >= lamports {
				return balance, nil
			}
			sleep(time.Second)
		}
		lastErr = fmt.Errorf("airdrop not confirmed after polling")
	}
	return 0, fmt.Errorf("init: airdrop failed after retries: %v", lastErr)
}

// OperatorKey resolves the vault authority key per call: flag > env, never
// written to the orbit home. The path may point at a keypair JSON (the
// Solana secret file) or a raw base58 64-byte secret line.
func OperatorKey(getenv func(string) string, flagKey, flagPath string) (sol.Keypair, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	secret := strings.TrimSpace(flagKey)
	if secret == "" {
		secret = strings.TrimSpace(getenv("ORBIT_OPERATOR_KEY"))
	}
	path := strings.TrimSpace(flagPath)
	if path == "" {
		path = strings.TrimSpace(getenv("ORBIT_OPERATOR_KEY_PATH"))
	}
	if secret == "" && path != "" {
		if strings.HasPrefix(path, "~") {
			home, err := os.UserHomeDir()
			if err != nil {
				return sol.Keypair{}, fmt.Errorf("operator key path: %w", err)
			}
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return sol.Keypair{}, fmt.Errorf("operator key %s: %w", path, err)
		}
		secret = strings.TrimSpace(string(b))
	}
	if secret == "" {
		return sol.Keypair{}, errors.New("operator key required: -operator-key, -operator-key-path, or ORBIT_OPERATOR_KEY(_PATH)")
	}
	kp, err := sol.KeypairFromSecret(secret)
	if err != nil {
		return sol.Keypair{}, fmt.Errorf("operator key: %w", err)
	}
	return kp, nil
}

// WriteConfigValue upserts a KEY=VALUE line in the config file (preserving
// the rest). Only public values are ever written (a pubkey, never a key).
func WriteConfigValue(path, key, value string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config write: %w", err)
	}
	lines := []string{}
	if len(b) > 0 {
		lines = strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
	found := false
	for i, line := range lines {
		k, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && k == key {
			lines[i] = key + "=" + value
			found = true
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
