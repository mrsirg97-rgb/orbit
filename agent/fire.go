package agent

import (
	"context"
	"fmt"

	"github.com/mrsirg97-rgb/rig/store/scheduler"
)

func Fire(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, jobID, rowID, brief, runnerCmd string, run func(context.Context) error) error {
	if brief == "" {
		return fmt.Errorf("agent: brief required")
	}
	if _, err := Refresh(ctx, db, ct, jobID, rowID, brief, "orbit-agent", runnerCmd); err != nil {
		return err
	}
	return run(ctx)
}
