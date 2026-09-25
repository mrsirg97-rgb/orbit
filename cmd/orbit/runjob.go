package main

import (
	"context"
	"fmt"
	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	orbittool "github.com/mrsirg97-rgb/orbit/tool"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
	"os"
	"path/filepath"
)

func runJobFire(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "orbit: usage: run-job <key>")
		return 2
	}
	// The per-fire brief: the job's agent row -> live snapshot -> brief
	// block. Fail closed: no identity, no read, no fire.
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
	brief, err := brief.Build(snap, size)
	if err != nil {
		die("run-job: brief: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		die("%v", err)
	}
	// The scheduler's home is ~/.rig/scheduler (DB, locks, run logs) — the
	// same path rig's own run-job uses.
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
		sandbox = "off" // the operator's choice; the agent runtime is unsandboxed by default
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
	if err := agent.Fire(ctx, sdb, sched.RealCrontab(""), args[0], row.ID, brief, self+" run-job", run); err != nil {
		fmt.Fprintln(os.Stderr, "orbit:", err)
		return 1
	}
	return 0
}

// ── agent: identity row → scheduled job ────────────────────────────────
