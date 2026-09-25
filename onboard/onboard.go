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

type Airdrop interface {
	RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error)
	GetBalance(ctx context.Context, pubkey string) (uint64, error)
}

type InitOpts struct {
	Home            string
	ConfigPath      string
	Getenv          func(string) string
	RPC             Airdrop
	AirdropLamports uint64
	AirdropBudget   time.Duration
	Sleep           func(time.Duration)
	Force           bool
}

type InitResult struct {
	Pubkey      string
	Balance     uint64
	KeyPath     string
	ConfigPath  string
	ReusedKey   bool
	AirdropOK   bool
	FundingHint string
	Next        []string
}

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
	reused := false
	var kp sol.Keypair
	if b, err := os.ReadFile(keyPath); err == nil && !opts.Force {

		kp, err = sol.KeypairFromSecret(strings.TrimSpace(string(b)))
		if err != nil {
			return InitResult{}, fmt.Errorf("init: existing key %s: %w", keyPath, err)
		}
		reused = true
	} else {
		kp, err = sol.GenerateKeypair()
		if err != nil {
			return InitResult{}, err
		}
		if err := os.WriteFile(keyPath, []byte(sol.Encode(kp.Secret)+"\n"), 0o600); err != nil {
			return InitResult{}, fmt.Errorf("init: write key: %w", err)
		}
	}

	existing, err := readConfig(cfgPath)
	if err != nil {
		return InitResult{}, err
	}
	indexer := firstNonEmpty(getenv("ORBIT_INDEXER"), existing["ORBIT_INDEXER"], client.DefaultIndexer)
	rpc := firstNonEmpty(getenv("ORBIT_RPC"), existing["ORBIT_RPC"], indexer+"/rpc")
	if isHostOnlyURL(rpc) {
		rpc += "/rpc"
	}
	values := map[string]string{
		"ORBIT_INDEXER":        indexer,
		"ORBIT_RPC":            rpc,
		"ORBIT_PROGRAM_ID":     firstNonEmpty(getenv("ORBIT_PROGRAM_ID"), existing["ORBIT_PROGRAM_ID"], client.DevnetProgramID),
		"ORBIT_AGENT_KEY_FILE": keyPath,
	}
	if creator := firstNonEmpty(getenv("ORBIT_VAULT_CREATOR"), existing["ORBIT_VAULT_CREATOR"], ""); creator != "" {
		values["ORBIT_VAULT_CREATOR"] = creator
	}
	if err := WriteConfigValues(cfgPath, values); err != nil {
		return InitResult{}, err
	}

	lamports := opts.AirdropLamports
	if lamports == 0 {
		lamports = 1_000_000_000
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	budget := opts.AirdropBudget
	if budget <= 0 {
		budget = 60 * time.Second
	}
	pub := kp.PublicBase58()
	balance, funded := airdropBounded(context.Background(), opts.RPC, pub, lamports, sleep, budget)
	hint := ""
	if !funded {
		hint = "faucet busy (the devnet airdrop limit is hit); fund it manually — send devnet SOL to " + pub + " or visit https://faucet.solana.com"
	}
	return InitResult{
		Pubkey:      pub,
		Balance:     balance,
		KeyPath:     keyPath,
		ConfigPath:  cfgPath,
		ReusedKey:   reused,
		AirdropOK:   funded,
		FundingHint: hint,
		Next: []string{
			"export ORBIT_OPERATOR_KEY_PATH=/path/to/your/operator.json",
			"orbit vault create     # the vault is created for your operator key",
			"orbit vault link " + pub,
			"orbit vault deposit 1",
			"orbit agent register --model <fleet-model>",
		},
	}, nil
}

func airdropBounded(ctx context.Context, rpc Airdrop, pubkey string, lamports uint64, sleep func(time.Duration), budget time.Duration) (uint64, bool) {
	if rpc == nil {
		return 0, false
	}
	start := time.Now()
	maxAttempts := int(budget / time.Second)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for attempt := 0; ; attempt++ {
		if _, err := rpc.RequestAirdrop(ctx, pubkey, lamports); err == nil {
			for poll := 0; poll < 4; poll++ {
				balance, berr := rpc.GetBalance(ctx, pubkey)
				if berr == nil && balance >= lamports {
					return balance, true
				}
				if time.Since(start) >= budget || attempt >= maxAttempts {
					balance, _ := rpc.GetBalance(ctx, pubkey)
					return balance, false
				}
				sleep(time.Second)
			}

		}
		if time.Since(start) >= budget || attempt >= maxAttempts {
			balance, _ := rpc.GetBalance(ctx, pubkey)
			return balance, false
		}

		backoff := time.Duration(2+attempt%3) * time.Second
		sleep(backoff)
	}
}

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

func WriteConfigValue(path, key, value string) error {
	return WriteConfigValues(path, map[string]string{key: value})
}

func WriteConfigValues(path string, values map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config write: %w", err)
	}
	lines := []string{}
	if len(b) > 0 {
		lines = strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
	for i, line := range lines {
		k, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			if v, has := values[k]; has {
				lines[i] = k + "=" + v
			}
		}
	}
	for k, v := range values {
		found := false
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), k+"=") {
				found = true
			}
		}
		if !found {
			lines = append(lines, k+"="+v)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func readConfig(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config read: %w", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) != "" {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func isHostOnlyURL(endpoint string) bool {
	slash := strings.Index(endpoint, "://")
	rest := endpoint
	if slash >= 0 {
		rest = endpoint[slash+3:]
	}
	return !strings.Contains(rest, "/")
}

func Load(getenv func(string) string) (map[string]string, error) {
	path, err := ConfigPath(getenv)
	if err != nil {
		return nil, err
	}
	return readConfig(path)
}
