// Package agent is the scheduled agent: one identity row → one rig scheduled
// job. The fire is rig -p with the world block as the brief; cadence comes
// from the identity row; stall/budget/timeout ride along as configured.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/rig/store/scheduler"
)

// JobName is the stable job name for the agent (the scheduler enforces one
// job per name).
func JobName(id string) string { return "orbit-agent-" + id }

// Register creates (or refreshes) the scheduled job from the identity row.
// The prompt is the world block; the cadence comes from the row.
func Register(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, row identity.Row, worldBlock, runnerCmd, sessionCwd, session string) (string, error) {
	if row.ID == "" {
		return "", fmt.Errorf("agent: identity row required")
	}
	if worldBlock == "" {
		return "", fmt.Errorf("agent: world block required")
	}
	return scheduler.Create(ctx, db, ct, scheduler.CreateInput{
		Name:    JobName(row.ID),
		Prompt:  worldBlock,
		Cron:    row.Cadence,
		Cwd:     sessionCwd,
		Model:   row.Model,
		Stall:   row.Stall,
		Budget:  row.Budget,
		Timeout: row.Timeout,
	}, sessionCwd, session, runnerCmd, time.Now)
}

// Refresh updates the job's prompt (the fresh world block) without touching
// the cadence or the budget.
func Refresh(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, jobID, rowID, worldBlock, session, runnerCmd string) (string, error) {
	if worldBlock == "" {
		return "", fmt.Errorf("agent: world block required")
	}
	return scheduler.Update(ctx, db, ct, scheduler.UpdateInput{
		ID: jobID, Name: JobName(rowID), Prompt: worldBlock,
	}, session, runnerCmd, time.Now)
}
