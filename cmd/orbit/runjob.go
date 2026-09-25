package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/earn"
	orbittool "github.com/mrsirg97-rgb/orbit/tool"
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
	snap, err := orbittool.Snapshot(ctx, tc, brief.Identity{Name: row.Name, Bio: row.Bio})
	if err != nil {
		die("run-job: brief: %v", err)
	}
	size := brief.Compact
	if row.BlockSize == "full" {
		size = brief.Full
	}
	text, err := brief.Build(snap, size)
	if err != nil {
		die("run-job: brief: %v", err)
	}

	writeStatusSnapshot(ctx, tc, snap)
	self, err := os.Executable()
	if err != nil {
		die("%v", err)
	}

	home := filepath.Join(mustRigHome(), "scheduler")
	if err := os.MkdirAll(home, 0o755); err != nil {
		die("%v", err)
	}
	swapURL := os.Getenv("RIG_SWAP_URL")
	if swapURL == "" {
		swapURL = "http://127.0.0.1:8090"
	}
	sandbox := os.Getenv("ORBIT_SANDBOX")
	if sandbox == "" {
		sandbox = "off"
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
			RigHome:   mustRigHome(),
			StateDir:  filepath.Join(mustRigHome(), "sessions"),
		})
	}
	if err := agent.Fire(ctx, sdb, sched.RealCrontab(""), args[0], row.ID, text, self+" run-job", run); err != nil {
		fmt.Fprintln(os.Stderr, "orbit:", err)
		return 1
	}
	return 0
}

func writeStatusSnapshot(ctx context.Context, tc *client.TorchClient, read brief.ReadState) {
	bdb, err := board.Open(board.StorePath(mustRigHome()))
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
	if err := earn.WriteSnapshot(earn.SnapshotPath(mustRigHome()), rows); err != nil {
		fmt.Fprintf(os.Stderr, "orbit: status snapshot: %v\n", err)
	}
}
