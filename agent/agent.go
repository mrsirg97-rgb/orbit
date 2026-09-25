// Package agent is the scheduled agent: one identity row → one rig scheduled
// job. The fire is rig -p with the brief; cadence comes
// from the identity row; stall/budget/timeout ride along as configured.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/rig/store/scheduler"
)

// JobPrefix is the stable job-name prefix for an agent (the scheduler
// enforces one job per name).
const JobPrefix = "orbit-agent-"

// JobName is the stable job name for the agent.
func JobName(id string) string { return JobPrefix + id }

// JobAgentID reverses JobName; a job whose name lacks the prefix is not an
// orbit agent job.
func JobAgentID(jobName string) string {
	if !strings.HasPrefix(jobName, JobPrefix) {
		return ""
	}
	return strings.TrimPrefix(jobName, JobPrefix)
}

// Register creates (or refreshes) the scheduled job from the identity row.
// The prompt is the brief; the cadence comes from the row.
func Register(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, row identity.Row, brief, runnerCmd, sessionCwd, session string) (string, error) {
	if row.ID == "" {
		return "", fmt.Errorf("agent: identity row required")
	}
	if brief == "" {
		return "", fmt.Errorf("agent: brief required")
	}
	return scheduler.Create(ctx, db, ct, scheduler.CreateInput{
		Name:    JobName(row.ID),
		Prompt:  brief,
		Cron:    row.Cadence,
		Cwd:     sessionCwd,
		Model:   row.Model,
		Stall:   row.Stall,
		Budget:  row.Budget,
		Timeout: row.Timeout,
	}, sessionCwd, session, runnerCmd, time.Now)
}

// Refresh updates the job's prompt (the fresh brief) without touching
// the cadence or the budget.
func Refresh(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, jobID, rowID, brief, session, runnerCmd string) (string, error) {
	if brief == "" {
		return "", fmt.Errorf("agent: brief required")
	}
	return scheduler.Update(ctx, db, ct, scheduler.UpdateInput{
		ID: jobID, Name: JobName(rowID), Prompt: brief,
	}, session, runnerCmd, time.Now)
}
