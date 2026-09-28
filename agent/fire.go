package agent

import (
	"context"
	"fmt"

	"github.com/mrsirg97-rgb/rig/v2/store/scheduler"
)

func Fire(ctx context.Context, db scheduler.DB, ct scheduler.Crontab, jobID, rowID, brief, runnerCmd, home string, run func(context.Context) error) error {
	if brief == "" {
		return fmt.Errorf("agent: brief required")
	}
	if _, err := Refresh(ctx, db, ct, jobID, rowID, brief, "orbit-agent", runnerCmd, home); err != nil {
		return err
	}
	return run(ctx)
}
