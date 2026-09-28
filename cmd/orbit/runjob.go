package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	rigconfig "github.com/mrsirg97-rgb/rig/v2/config"
	"github.com/mrsirg97-rgb/rig/v2/store"
	sched "github.com/mrsirg97-rgb/rig/v2/store/scheduler"

	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/earn"
	"github.com/mrsirg97-rgb/orbit/identity"
	orbittool "github.com/mrsirg97-rgb/orbit/tool"
)

func runJobFire(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "orbit: usage: run-job <key>")
		return 2
	}

	ctx := context.Background()
	sdb := schedStore()
	defer sdb.DB.Close()
	row, err := jobAgentRow(ctx, sdb, args[0])
	if err != nil {
		die("run-job: %v", err)
	}
	os.Setenv("ORBIT_AGENT_ID", row.ID)
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("run-job: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("run-job: %v", err)
	}
	if err := checkFireFunded(ctx, tc); err != nil {
		die("%v", err)
	}
	snap, text, err := fireBrief(ctx, tc, row)
	if err != nil {
		die("run-job: brief: %v", err)
	}
	writeStatusSnapshot(ctx, tc, snap)
	self, err := os.Executable()
	if err != nil {
		die("%v", err)
	}

	home := filepath.Join(mustOrbitHome(), "scheduler")
	if err := os.MkdirAll(home, 0o755); err != nil {
		die("%v", err)
	}
	if err := writeFireAgentID(mustOrbitHome(), row.ID); err != nil {
		die("run-job: %v", err)
	}
	sandbox, swapURL, err := fireSandboxSwap(os.Getenv)
	if err != nil {
		die("run-job: settings: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mustOrbitHome(), "kernel"), 0o755); err != nil {
		die("run-job: sandbox kernel: %v", err)
	}
	tunnel, err := startChainTunnel(fireChainSock(mustOrbitHome()), "443", fireTunnelHosts(cfg))
	if err != nil {
		die("run-job: chain tunnel: %v", err)
	}
	defer tunnel.Close()
	run := func(ctx context.Context) error {
		return sched.RunJob(args[0], sched.RunOpts{
			Home:      home,
			Crontab:   sched.RealCrontab(""),
			Fetch:     sched.RealFetch(0),
			Spawn:     sched.RealSpawn,
			WorkerCmd: []string{self},
			SwapURL:   swapURL,
			Sandbox:   sandbox,
			RigHome:   mustOrbitHome(),
			StateDir:  filepath.Join(mustOrbitHome(), "sessions"),
		})
	}
	if err := agent.Fire(ctx, sdb, sched.RealCrontab(""), args[0], row.ID, text, agent.RunnerCommand(self), mustOrbitHome(), run); err != nil {
		writeFireEndStatus(ctx, tc)
		fmt.Fprintln(os.Stderr, "orbit:", err)
		return 1
	}
	writeFireEndStatus(ctx, tc)
	return 0
}

func fireBrief(ctx context.Context, tc *client.TorchClient, row identity.Row) (brief.ReadState, string, error) {
	// The architect's goal rides the live brief: Name and Bio alone drop it.
	snap, err := orbittool.Snapshot(ctx, tc, brief.Identity{Name: row.Name, Bio: row.Bio, Goal: row.Goal})
	if err != nil {
		return brief.ReadState{}, "", err
	}
	size := brief.Compact
	if row.BlockSize == "full" {
		size = brief.Full
	}
	text, err := brief.Build(snap, size)
	if err != nil {
		return brief.ReadState{}, "", err
	}
	return snap, text, nil
}

func fireSandboxSwap(getenv func(string) string) (string, string, error) {
	return fireSandboxSwapWith(getenv, func() error { _, err := sched.LandlockABI(); return err }, os.Stderr)
}

// fireSandboxSwapWith picks the fire's sandbox. Landlock, the netless
// profile, whenever the kernel has it: the operator's interactive setting
// never turns it off. Where the box cannot provide it (macOS, an old
// kernel) the fire runs with the operator's configured sandbox and says so
// in one line; a box without landlock is a first-class box, never a
// refused fire. The worker swap URL comes from settings.json with the env
// override either way.
func fireSandboxSwapWith(getenv func(string) string, probe func() error, notice io.Writer) (string, string, error) {
	home, err := client.Home(getenv)
	if err != nil {
		return "", "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	cfg, err := rigconfig.Load(home, cwd)
	if err != nil {
		return "", "", err
	}
	swapURL := cfg.Settings.SwapURL
	if v := strings.TrimSpace(getenv("RIG_SWAP_URL")); v != "" {
		swapURL = v
	}
	if err := probe(); err != nil {
		fallback := strings.TrimSpace(cfg.Settings.Sandbox)
		if fallback == "" || fallback == "landlock" {
			fallback = "off"
		}
		if notice != nil {
			fmt.Fprintf(notice, "orbit: fire sandbox: landlock unavailable (%v); running with sandbox %q\n", err, fallback)
		}
		return fallback, swapURL, nil
	}
	return "landlock", swapURL, nil
}

// fireTunnelHosts are the only TLS server names the chain tunnel forwards:
// the indexer, the RPC seam, and the devnet airdrop RPC. Anything else the
// jail asks for is refused and logged, so the netless sandbox stays
// meaningful with a shell inside it.
func fireTunnelHosts(cfg client.Config) []string {
	var hosts []string
	for _, raw := range []string{cfg.Indexer, cfg.RPC, client.DevnetAirdropRPC} {
		if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	return hosts
}

func checkFireFunded(ctx context.Context, tc *client.TorchClient) error {
	balance, err := tc.RPC.GetBalance(ctx, tc.AgentPublic())
	if err != nil {
		return fmt.Errorf("run-job: hot wallet balance: %w", err)
	}
	if balance < earn.FundingFloorLamports {
		return fmt.Errorf("run-job: %s", earn.FundingLine(tc.AgentPublic()))
	}
	return nil
}

func writeFireAgentID(home, agentID string) error {
	dir := filepath.Join(home, ".rig-job")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("run-job: fire scratch: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "agent-id"), []byte(agentID), 0o644)
}

func writeStatusSnapshot(ctx context.Context, tc *client.TorchClient, read brief.ReadState) {
	bdb, err := board.Open(board.StorePath(mustOrbitHome()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
		return
	}
	defer bdb.DB.Close()
	st := &board.Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: bdb}
	rows, err := earn.RowsFromBrief(ctx, read, tc, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
		return
	}
	writeSnapshotRows(rows)
}

func writeFireEndStatus(ctx context.Context, tc *client.TorchClient) {
	bdb, err := board.Open(board.StorePath(mustOrbitHome()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
		return
	}
	defer bdb.DB.Close()
	st := &board.Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: bdb}
	rows, err := earn.Status(ctx, tc, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
		return
	}
	writeSnapshotRows(rows)
}

func writeSnapshotRows(rows earn.Rows) {
	if err := earn.WriteSnapshot(earn.SnapshotPath(mustOrbitHome()), rows, earn.Fire{}); err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
	}
}

func fireActSnapshot(ctx context.Context, idb store.DB, tc *client.TorchClient, st *board.Store, path, agentID string, shape board.Shape) {
	row, err := identity.GetByID(ctx, idb, agentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orbit: fire status: %v\n", err)
		return
	}
	rows, err := earn.Status(ctx, tc, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orbit: fire status: %v\n", err)
		return
	}
	fire := earn.Fire{Role: row.Role, Verb: shape.Verb, Task: shape.ID, At: time.Now().UTC().Format(time.RFC3339)}
	if err := earn.WriteSnapshot(path, rows, fire); err != nil {
		fmt.Fprintf(os.Stderr, "orbit: fire status: %v\n", err)
	}
}
