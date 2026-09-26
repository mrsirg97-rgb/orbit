package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/earn"
	"github.com/mrsirg97-rgb/orbit/identity"
	orbittool "github.com/mrsirg97-rgb/orbit/tool"
	rigconfig "github.com/mrsirg97-rgb/rig/config"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
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
	sandbox, swapURL, err := fireSandboxSwap(os.Getenv)
	if err != nil {
		die("run-job: settings: %v", err)
	}
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
		fmt.Fprintln(os.Stderr, "orbit:", err)
		return 1
	}
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
	// The sandbox and the worker swap URL come from settings.json like main
	// reads them; env overrides. Neither is hardcoded here.
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
	sandbox := cfg.Settings.Sandbox
	if v := strings.TrimSpace(getenv("ORBIT_SANDBOX")); v != "" {
		sandbox = v
	}
	if sandbox == "" {
		sandbox = "off"
	}
	swapURL := cfg.Settings.SwapURL
	if v := strings.TrimSpace(getenv("RIG_SWAP_URL")); v != "" {
		swapURL = v
	}
	return sandbox, swapURL, nil
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
	if err := earn.WriteSnapshot(earn.SnapshotPath(mustOrbitHome()), rows); err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
	}
}
