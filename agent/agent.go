package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/rig/store/scheduler"
)

const JobPrefix = "orbit-agent-"

func JobName(id string) string { return JobPrefix + id }

func JobAgentID(jobName string) string {
	if !strings.HasPrefix(jobName, JobPrefix) {
		return ""
	}
	return strings.TrimPrefix(jobName, JobPrefix)
}

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

// RunnerCommand builds the cron runner command with the executable shell-
// quoted: a path with spaces or shell metacharacters must not break the
// crontab line.
func RunnerCommand(executable string) string {
	return shellQuote(executable) + " run-job"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func Refresh(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, jobID, rowID, brief, session, runnerCmd string) (string, error) {
	if brief == "" {
		return "", fmt.Errorf("agent: brief required")
	}
	return scheduler.Update(ctx, db, ct, scheduler.UpdateInput{
		ID: jobID, Name: JobName(rowID), Prompt: brief,
	}, session, runnerCmd, time.Now)
}
