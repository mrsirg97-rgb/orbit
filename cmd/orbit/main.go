// orbit is the torch agent runtime: a rig one-shot worker with the market /
// intel / wallet tools, one scheduled agent (identity row → job), and the
// vault bootstrap printer. The operator's vault key never enters the process.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mrsirg97-rgb/rig"
	"github.com/mrsirg97-rgb/rig/core"
	"github.com/mrsirg97-rgb/rig/frontend/oneshot"
	"github.com/mrsirg97-rgb/rig/loop"
	"github.com/mrsirg97-rgb/rig/policy"
	"github.com/mrsirg97-rgb/rig/provider/openai"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
	scheddomain "github.com/mrsirg97-rgb/rig/store/scheduler/domain"

	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/tool"
	"github.com/mrsirg97-rgb/orbit/world"
	"github.com/mrsirg97-rgb/rig/store"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "run-job":
			os.Exit(runJob(args[1:]))
		case "agent":
			os.Exit(runAgent(args[1:]))
		case "snapshot":
			os.Exit(runSnapshot(args[1:]))
		case "bootstrap":
			os.Exit(runBootstrap(args[1:]))
		case "init":
			os.Exit(runInit(args[1:]))
		case "vault":
			os.Exit(runVault(args[1:]))
		case "project":
			os.Exit(runProject(args[1:]))
		case "board":
			os.Exit(runBoard(args[1:]))
		}
	}
	os.Exit(runWorker(args))
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "orbit: "+format+"\n", a...)
	os.Exit(1)
}

// ── the worker (rig one-shot with the orbit tools) ─────────────────────

func runWorker(args []string) int {
	fs := flag.NewFlagSet("orbit", flag.ContinueOnError)
	prompt := fs.String("p", "", "one-shot: run the single prompt and exit (the scheduler's worker path)")
	sessionID := fs.String("session-id", "", "set the fresh session identity (worker use)")
	baseURL := fs.String("base-url", "", "OpenAI-compatible endpoint base URL (the worker swap)")
	model := fs.String("model", "", "model name")
	system := fs.String("system", "", "system prompt")
	allow := fs.String("allow", "", "comma-separated allow-list of tool names")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *prompt == "" {
		fmt.Fprintln(os.Stderr, "orbit: worker mode needs -p (or a subcommand)")
		return 2
	}
	if *baseURL == "" {
		die("worker: no -base-url")
	}
	if *model == "" {
		die("worker: no -model")
	}
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("%v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("%v", err)
	}
	role, stake := workerIdentity()
	bs, err := board.Open(board.StorePath(rigHome()))
	if err != nil {
		die("board store: %v", err)
	}
	defer bs.DB.Close()
	tools := []core.Tool{
		&tool.Market{Client: tc, Role: role, StakeLamports: stake},
		&tool.Intel{Client: tc},
		&tool.Wallet{Client: tc},
		&tool.Board{Store: &board.Store{Client: tc, DB: bs, Role: role}, Client: tc, Role: role},
	}
	if *allow != "" {
		allowed := map[string]bool{}
		for _, name := range strings.Split(*allow, ",") {
			allowed[strings.TrimSpace(name)] = true
		}
		filtered := tools[:0]
		for _, t := range tools {
			if allowed[t.Name()] {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}
	_ = sessionID
	k := rig.New(
		rig.WithProvider(openai.New(*baseURL, *model)),
		rig.WithFrontend(&oneshot.OneShot{Prompt: *prompt, Out: os.Stdout, Err: os.Stderr}),
		rig.WithPolicy(policy.Passthrough(*system)),
		rig.WithTools(tools...),
	)
	if err := loop.Run(context.Background(), k); err != nil {
		fmt.Fprintf(os.Stderr, "orbit: worker: %v\n", err)
		return 1
	}
	return 0
}

// ── run-job: the scheduler's fire path ─────────────────────────────────

func runJob(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "orbit: usage: run-job <key>")
		return 2
	}
	// The per-fire brief: the job's agent row -> live snapshot -> world
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
	d, err := identity.DefaultsFor(identity.Role(row.Role))
	if err != nil {
		die("run-job: %v", err)
	}
	snap, err := tool.Snapshot(ctx, tc, world.Identity{
		Name: row.Name, Bio: row.Bio, Personality: row.Personality,
		Role: row.Role, Directive: d.Directive, MemoShapes: d.MemoShapes,
		Stake: identity.StakeLamports(row.StakeScale), Voice: row.Voice,
	})
	if err != nil {
		die("run-job: brief: %v", err)
	}
	size := world.Compact
	if row.BlockSize == "full" {
		size = world.Full
	}
	brief, err := world.Build(snap, size)
	if err != nil {
		die("run-job: brief: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		die("%v", err)
	}
	// The scheduler's home is ~/.rig/scheduler (DB, locks, run logs) — the
	// same path rig's own run-job uses.
	home := filepath.Join(rigHome(), "scheduler")
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
			RigHome:   rigHome(),
			StateDir:  filepath.Join(rigHome(), "sessions"),
		})
	}
	if err := agent.Fire(ctx, sdb, sched.RealCrontab(""), args[0], row.ID, brief, self+" run-job", run); err != nil {
		fmt.Fprintln(os.Stderr, "orbit:", err)
		return 1
	}
	return 0
}

// ── agent: identity row → scheduled job ────────────────────────────────

func runAgent(args []string) int {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	aaction := fs.String("action", "register", "register | refresh | show | list")
	roleFlag := fs.String("role", "", "architect | worker | reviewer (required for register)")
	voice := fs.String("voice", "", "loyalist | mercenary | provocateur | scout | whale (colors the memo tone only)")
	name := fs.String("name", "", "agent display name (default from role)")
	bio := fs.String("bio", "", "agent bio")
	personality := fs.String("personality", "mercenary", "loyalist | mercenary | provocateur | scout | whale")
	cadence := fs.String("cadence", "", "5-field cron (default from role)")
	model := fs.String("model", "", "worker model (required)")
	stall := fs.Int("stall", 0, "stall minutes (0 = role default)")
	budget := fs.Float64("budget", 0, "dollar budget cap (0 = role default)")
	timeout := fs.Int("timeout", 0, "timeout minutes (0 = role default)")
	full := fs.Bool("full", false, "register with the full world block")
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
		role, err := identity.ParseRole(*roleFlag)
		if err != nil {
			die("%v", err)
		}
		if *model == "" {
			die("agent: -model required")
		}
		row, err := identity.NewRow(tc.AgentPublic(), role, identity.Overrides{
			Name: *name, Bio: *bio, Personality: *personality, Voice: *voice,
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
		fmt.Printf("%-22s %-10s %-16s %-8s %-12s %-8s %-8s %s\n", "ID", "ROLE", "NAME", "MODEL", "CADENCE", "BLOCK", "BUDGET", "JOB")
		for _, row := range rows {
			jobID := findJobID(ctx, sdb, agent.JobName(row.ID))
			if jobID == "" {
				jobID = "-"
			}
			fmt.Printf("%-22s %-10s %-16s %-8s %-12s %-8s $%-7.2f %s\n",
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
	// The stored prompt is a stub naming the identity and its role; the live
	// world block is rebuilt per fire by run-job.
	d, err := identity.DefaultsFor(identity.Role(row.Role))
	if err != nil {
		die("%v", err)
	}
	block := world.StubBlock(world.Identity{
		Name: row.Name, Bio: row.Bio, Personality: row.Personality,
		Role: row.Role, Directive: d.Directive, MemoShapes: d.MemoShapes,
		Stake: identity.StakeLamports(row.StakeScale), Voice: row.Voice,
	})
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
	fmt.Printf("VOICE      %s\n", row.Voice)
	fmt.Printf("PERSONALITY %s\n", row.Personality)
	fmt.Printf("BIO        %s\n", row.Bio)
	fmt.Printf("CADENCE    %s\n", row.Cadence)
	fmt.Printf("MODEL      %s\n", row.Model)
	fmt.Printf("BLOCK      %s\n", row.BlockSize)
	fmt.Printf("BUDGET     $%.2f\n", row.Budget)
	fmt.Printf("STALL      %dm\n", row.Stall)
	fmt.Printf("TIMEOUT    %dm\n", row.Timeout)
	fmt.Printf("STAKE      %.2fx (%s SOL per action)\n", row.StakeScale, client.FormatSOL(identity.StakeLamports(row.StakeScale)))
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
	job, err := scheddomain.NewJobDomain().GetJob(bound, jobID).Row()
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
	job, err := scheddomain.NewJobDomain().GetJob(bound, key).Row()
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

// ── snapshot: print the world block ────────────────────────────────────

func runSnapshot(args []string) int {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	full := fs.Bool("full", false, "the full block (default compact)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("%v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("%v", err)
	}
	snap, err := tool.Snapshot(context.Background(), tc, world.Identity{Name: "@AP" + walletSuffix(tc.AgentPublic()), Bio: "torch agent", Personality: "mercenary", Role: "worker", Directive: "claims and completes tasks", MemoShapes: "claim | note | complete", Stake: identity.StakeLamports(0.25), Voice: "mercenary"})
	if err != nil {
		die("snapshot: %v", err)
	}
	size := world.Compact
	if *full {
		size = world.Full
	}
	block, err := world.Build(snap, size)
	if err != nil {
		die("snapshot: %v", err)
	}
	fmt.Print(block)
	fmt.Fprintf(os.Stderr, "orbit: %d tokens (%d bytes)\n", world.Tokens(block), len(block))
	return 0
}

// ── bootstrap: unsigned vault admin for the operator ───────────────────

func runBootstrap(args []string) int {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	deposit := fs.Int64("deposit", 0, "lamports to deposit into the vault")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("%v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("%v", err)
	}
	fmt.Fprintf(os.Stderr, "orbit: vault %s (creator %s), link %s\n", tc.VaultPDA(), tc.VaultCreator, tc.AgentPublic())
	fmt.Fprintln(os.Stderr, "orbit: sign each line with the OPERATOR's vault authority key; the process never holds it.")
	fmt.Fprintln(os.Stderr, "orbit: 1. create_vault  2. link_wallet  3. deposit_vault  4. done: re-run agent register.")
	out := map[string]UnsignedIx{
		"create_vault": bootstrapTx(tc, cfg, "create_vault", nil),
		"link_wallet":  bootstrapTx(tc, cfg, "link_wallet", nil),
	}
	if *deposit > 0 {
		out["deposit_vault"] = bootstrapTx(tc, cfg, "deposit_vault", map[string]any{"sol_amount": uint64(*deposit)})
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return 0
}

// ── helpers ────────────────────────────────────────────────────────────

func rigHome() string {
	if h := os.Getenv("RIG_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".rig")
}

func walletSuffix(pubkey string) string {
	s := pubkey
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return strings.ToUpper(s)
}

// workerIdentity resolves the worker's role and per-action stake. The
// scheduler fire sets ORBIT_AGENT_ID (run-job); direct -p use falls back to
// the newest row, and without any row the tools run untagged.
func workerIdentity() (string, uint64) {
	ctx := context.Background()
	idb := identityStore()
	defer idb.DB.Close()
	id := os.Getenv("ORBIT_AGENT_ID")
	var row identity.Row
	var err error
	if id != "" {
		row, err = identity.GetByID(ctx, idb, id)
		if err != nil {
			die("worker: identity %s: %v", id, err)
		}
	} else {
		row, err = identity.Get(ctx, idb)
		if err != nil {
			return "", 0
		}
	}
	return row.Role, identity.StakeLamports(row.StakeScale)
}
