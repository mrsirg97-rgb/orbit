package agent

import (
	"context"
	"fmt"

	"github.com/mrsirg97-rgb/rig/store/scheduler"
)

// Fire is the per-fire path: the caller builds the fresh brief (a live read
// snapshot projected through world.Build), Fire refreshes the job's prompt
// with it, then runs the fire. The world block is therefore per-fire — the
// prompt stored by register is only a stub naming the identity until the
// first fire replaces it.
//
// Register order (why a fire can skip with "no crontab line (drift)"):
//
//  1. identity upsert (one row, idempotent);
//  2. scheduler Create — it installs the crontab line FIRST, then commits the
//     job row. A concurrent fire that checks the line before the commit (or
//     after the line was pruned/replaced by another crontab edit) sees the
//     line missing and records the drift skip;
//  3. the fire's run-job re-asserts the line through Refresh (Update runs
//     UpsertLine + Install), then loads the job and spawns the worker.
//
// So: register → job row + line; every fire → brief rebuild → prompt update →
// line re-assert → spawn. The drift skip is the guard firing before the line
// lands, not a lost job.
func Fire(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, jobID, rowID, brief, runnerCmd string, run func(context.Context) error) error {
	if brief == "" {
		return fmt.Errorf("agent: brief required")
	}
	if _, err := Refresh(ctx, db, ct, jobID, rowID, brief, "orbit-agent", runnerCmd); err != nil {
		return err
	}
	return run(ctx)
}
