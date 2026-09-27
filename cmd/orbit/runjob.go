package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rigconfig "github.com/mrsirg97-rgb/rig/config"
	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"

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
	tunnel, err := startChainTunnel(fireChainSock(mustOrbitHome()), "443")
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
	if err := agent.Fire(ctx, sdb, sched.RealCrontab(""), args[0], row.ID, text, agent.RunnerCommand(self), run); err != nil {
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
	// The fire's sandbox is always on: the operator's interactive setting
	// never reaches a fire. Landlock is the netless profile and works
	// unprivileged; only the worker swap URL still comes from settings.json
	// with the env override.
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
	return "landlock", swapURL, nil
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
