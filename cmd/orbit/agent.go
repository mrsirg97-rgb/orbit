package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
	"github.com/mrsirg97-rgb/rig/store/scheduler/domain"
	"os"
)

func runAgent(args []string) int {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	aaction := fs.String("action", "register", "register | refresh | show | list")
	role := fs.String("role", "worker", "worker | architect | reviewer (register only)")
	name := fs.String("name", "", "agent display name")
	bio := fs.String("bio", "", "agent bio")
	cadence := fs.String("cadence", "", "5-field cron")
	model := fs.String("model", "", "worker model (required)")
	stall := fs.Int("stall", 0, "stall minutes")
	budget := fs.Float64("budget", 0, "dollar budget cap")
	timeout := fs.Int("timeout", 0, "timeout minutes")
	full := fs.Bool("full", false, "register with the full brief")
	fs.SetOutput(os.Stderr)
	// The action is the first positional; flags follow it (flag.Parse stops at
	// the first non-flag).
	if len(args) > 0 {
		*aaction = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx := context.Background()
	idb := identityStore()
	defer idb.DB.Close()
	switch *aaction {
	case "register":
		cfg, err := client.LoadConfig(os.Getenv)
		if err != nil {
			die("%v", err)
		}
		tc, err := client.New(cfg)
		if err != nil {
			die("%v", err)
		}
		if *model == "" {
			die("agent: -model required")
		}
		role, err := identity.ParseRole(*role)
		if err != nil {
			die("%v", err)
		}
		row, err := identity.NewRow(tc.AgentPublic(), role, identity.Overrides{
			Name: *name, Bio: *bio,
			Cadence: *cadence, Model: *model, Budget: *budget,
			Stall: *stall, Timeout: *timeout, Full: *full,
		})
		if err != nil {
			die("%v", err)
		}
		if err := identity.Upsert(ctx, idb, row); err != nil {
			die("%v", err)
		}
		return ensureJob(ctx, idb, row)
	case "refresh":
		rows, err := identity.List(ctx, idb)
		if err != nil {
			die("%v", err)
		}
		if len(rows) == 0 {
			die("agent: no identity row (register first)")
		}
		for _, row := range rows {
			if code := ensureJobAction(ctx, idb, row, "refresh"); code != 0 {
				return code
			}
		}
		return 0
	case "show":
		ids := fs.Args()
		if len(ids) != 1 {
			die("agent: usage: agent show <id>")
		}
		row, err := identity.GetByID(ctx, idb, ids[0])
		if err != nil {
			die("%v", err)
		}
		return printAgent(ctx, idb, row)
	case "list":
		rows, err := identity.List(ctx, idb)
		if err != nil {
			die("%v", err)
		}
		sdb := schedStore()
		defer sdb.DB.Close()
		fmt.Printf("%-24s %-10s %-16s %-8s %-12s %-8s %-8s %s\n", "ID", "ROLE", "NAME", "MODEL", "CADENCE", "BLOCK", "BUDGET", "JOB")
		for _, row := range rows {
			jobID := findJobID(ctx, sdb, agent.JobName(row.ID))
			if jobID == "" {
				jobID = "-"
			}
			fmt.Printf("%-24s %-10s %-16s %-8s %-12s %-8s $%-7.2f %s\n",
				row.ID, row.Role, row.Name, row.Model, row.Cadence, row.BlockSize, row.Budget, jobID)
		}
		return 0
	default:
		die("agent: unknown action %q", *aaction)
	}
	return 0
}

func ensureJob(ctx context.Context, idb store.DB, row identity.Row) int {
	return ensureJobAction(ctx, idb, row, "register")
}

func ensureJobAction(ctx context.Context, idb store.DB, row identity.Row, action string) int {
	// The stored prompt is a stub naming the identity; the live brief
	// is rebuilt per fire by run-job.
	block := brief.StubBrief(brief.Identity{Name: row.Name, Bio: row.Bio, Goal: row.Goal})
	self, err := os.Executable()
	if err != nil {
		die("%v", err)
	}
	sdb := schedStore()
	defer sdb.DB.Close()
	cwd, err := os.Getwd()
	if err != nil {
		die("%v", err)
	}
	var reply string
	if action == "refresh" {
		jobID := findJobID(ctx, sdb, agent.JobName(row.ID))
		if jobID == "" {
			die("agent: job: no existing job %s (register first)", agent.JobName(row.ID))
		}
		reply, err = agent.Refresh(ctx, sdb, sched.RealCrontab(""), jobID, row.ID, block, "orbit-agent", self+" run-job")
	} else {
		reply, err = agent.Register(ctx, sdb, sched.RealCrontab(""), row, block, self+" run-job", cwd, "orbit-agent")
	}
	if err != nil {
		die("agent: job: %v", err)
	}
	fmt.Println(reply)
	return 0
}

func printAgent(ctx context.Context, idb store.DB, row identity.Row) int {
	fmt.Printf("AGENT      %s\n", row.ID)
	fmt.Printf("NAME       %s\n", row.Name)
	fmt.Printf("WALLET     %s\n", row.Wallet)
	fmt.Printf("ROLE       %s\n", row.Role)
	fmt.Printf("GOAL       %s\n", row.Goal)
	fmt.Printf("BIO        %s\n", row.Bio)
	fmt.Printf("CADENCE    %s\n", row.Cadence)
	fmt.Printf("MODEL      %s\n", row.Model)
	fmt.Printf("BLOCK      %s\n", row.BlockSize)
	fmt.Printf("BUDGET     $%.2f\n", row.Budget)
	fmt.Printf("STALL      %dm\n", row.Stall)
	fmt.Printf("TIMEOUT    %dm\n", row.Timeout)
	fmt.Printf("CREATED    %s\n", row.CreatedAt)
	sdb := schedStore()
	defer sdb.DB.Close()
	jobID := findJobID(ctx, sdb, agent.JobName(row.ID))
	if jobID == "" {
		fmt.Println("JOB        none (register first)")
		return 0
	}
	bound, tx, err := sdb.TxReadOnly(ctx)
	if err != nil {
		die("agent: %v", err)
	}
	job, err := domain.NewJobDomain().GetJob(bound, jobID).Row()
	tx.Rollback()
	if err != nil {
		die("agent: %v", err)
	}
	if job == nil {
		fmt.Println("JOB        missing row")
		return 0
	}
	fmt.Printf("JOB        %s\n", jobID)
	fmt.Printf("JOB NAME   %s\n", job.Name)
	fmt.Printf("JOB CRON   %s\n", job.Cron)
	fmt.Printf("JOB MODEL  %s\n", job.Model)
	jobBudget := 0.0
	if job.Budget != nil {
		jobBudget = *job.Budget
	}
	fmt.Printf("JOB BUDGET $%.2f\n", jobBudget)
	jobStall := int64(0)
	if job.Stall != nil {
		jobStall = *job.Stall
	}
	fmt.Printf("JOB STALL  %dm\n", jobStall)
	jobTimeout := int64(0)
	if job.Timeout != nil {
		jobTimeout = *job.Timeout
	}
	fmt.Printf("JOB TIMEOUT %dm\n", jobTimeout)
	return 0
}

func findJobID(ctx context.Context, db sched.DB, name string) string {
	var id string
	err := db.DB.QueryRowContext(ctx, `SELECT id FROM jobs WHERE name = ? ORDER BY rowid DESC LIMIT 1`, name).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func jobAgentRow(ctx context.Context, db sched.DB, key string) (identity.Row, error) {
	bound, tx, err := db.TxReadOnly(ctx)
	if err != nil {
		return identity.Row{}, err
	}
	job, err := domain.NewJobDomain().GetJob(bound, key).Row()
	tx.Rollback()
	if err != nil {
		return identity.Row{}, err
	}
	if job == nil {
		return identity.Row{}, fmt.Errorf("run-job: job %s not found", key)
	}
	id := agent.JobAgentID(job.Name)
	if id == "" {
		return identity.Row{}, fmt.Errorf("run-job: job %s is not an orbit agent job", key)
	}
	idb := identityStore()
	defer idb.DB.Close()
	return identity.GetByID(ctx, idb, id)
}
