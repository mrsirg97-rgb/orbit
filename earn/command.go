package earn

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
	scheddomain "github.com/mrsirg97-rgb/rig/store/scheduler/domain"

	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/onboard"
	"github.com/mrsirg97-rgb/orbit/sol"
)

type Command struct {
	Getenv    func(string) string
	Client    func() (*client.TorchClient, error)
	Init      func() (onboard.InitResult, error)
	Operator  func(flagKey, flagPath string) (sol.Keypair, error)
	NewClient func(client.Config) (*client.TorchClient, error)

	IdentityDB store.DB
	SchedDB    sched.DB
	Crontab    sched.Crontab
	Board      *board.Store

	Self         string
	Cwd          string
	Session      string
	Model        func() string
	SnapshotPath string
}

func (c *Command) Name() string { return "earn" }

func (c *Command) Description() string {
	return "join: check the hot key and the vault link, run init and the vault steps when missing, register roles (architect, worker, reviewer) and start the jobs; status, stop, start"
}

func (c *Command) Run(ctx context.Context, args string, env any) (string, error) {
	in, err := parseArgs(args)
	if err != nil {
		return "", err
	}
	switch in.action {
	case "status":
		return c.status(ctx)
	case "stop":
		return c.stop(ctx, false)
	case "start":
		return c.stop(ctx, true)
	}
	return c.register(ctx, in)
}

func (c *Command) status(ctx context.Context) (string, error) {
	tc, err := c.client(ctx)
	if err != nil {
		return "", err
	}
	rows, err := Status(ctx, tc, c.Board)
	if err != nil {
		return "", err
	}
	if c.SnapshotPath != "" {
		if err := WriteSnapshot(c.SnapshotPath, rows); err != nil {
			return "", err
		}
	}
	return strings.Join(rows.Lines(), "\n"), nil
}

func (c *Command) stop(ctx context.Context, resume bool) (string, error) {
	rows, err := identity.List(ctx, c.IdentityDB)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "earn: no identity rows (register first)", nil
	}
	var lines []string
	verb := "paused"
	if resume {
		verb = "resumed"
	}
	for _, row := range rows {
		id := findJobID(ctx, c.SchedDB, agent.JobName(row.ID))
		if id == "" {
			lines = append(lines, fmt.Sprintf("%s: no job", row.ID))
			continue
		}
		var reply string
		if resume {
			reply, err = sched.Resume(ctx, c.SchedDB, c.Crontab, id, c.Cwd, c.Session)
		} else {
			reply, err = sched.Pause(ctx, c.SchedDB, c.Crontab, id, c.Cwd, c.Session)
		}
		if err != nil {
			return "", fmt.Errorf("earn: %s %s: %w", verb, row.ID, err)
		}
		lines = append(lines, fmt.Sprintf("%s: %s", row.ID, reply))
	}
	return "earn: " + verb + "\n" + strings.Join(lines, "\n"), nil
}

func (c *Command) register(ctx context.Context, in args) (string, error) {
	if len(in.roles) == 0 {
		return "", fmt.Errorf("earn: which roles? architect, worker, reviewer (any mix) — e.g. /earn architect worker --goal \"<one paragraph>\" --operator-key-path /path")
	}
	for _, role := range in.roles {
		if role == identity.Architect && strings.TrimSpace(in.goal) == "" {
			return "", fmt.Errorf("earn: architect needs a goal: add goal \"<one paragraph>\" (the goal rides the brief)")
		}
	}
	// The model check runs before any chain spend: a run without a model
	// must not airdrop, create, link, or deposit.
	model := strings.TrimSpace(in.model)
	if model == "" && c.Model != nil {
		model = c.Model()
	}
	if model == "" {
		return "", fmt.Errorf("earn: no model: add --model <model> (or run orbit with one)")
	}
	tc, steps, err := c.ensureSetup(ctx, in)
	if err != nil {
		return "", err
	}
	var rows []identity.Row
	var lines []string
	for _, role := range in.roles {
		row, err := identity.NewRow(tc.AgentPublic(), role, identity.Overrides{Model: model, Goal: in.goal})
		if err != nil {
			return "", err
		}
		if err := identity.Upsert(ctx, c.IdentityDB, row); err != nil {
			return "", fmt.Errorf("earn: register %s: %w", role, err)
		}
		rows = append(rows, row)
		stub := brief.StubBrief(brief.Identity{Name: row.Name, Bio: row.Bio, Goal: row.Goal})
		var reply string
		if id := findJobID(ctx, c.SchedDB, agent.JobName(row.ID)); id != "" {
			reply, err = agent.Refresh(ctx, c.SchedDB, c.Crontab, id, row.ID, stub, c.Session, c.runner())
		} else {
			jobCwd, cerr := c.JobCwd()
			if cerr != nil {
				return "", fmt.Errorf("earn: job cwd: %w", cerr)
			}
			reply, err = agent.Register(ctx, c.SchedDB, c.Crontab, row, stub, c.runner(), jobCwd, c.Session)
		}
		if err != nil {
			return "", fmt.Errorf("earn: job %s: %w", row.ID, err)
		}
		lines = append(lines, fmt.Sprintf("%s: %s", row.ID, reply))
	}
	roster := Roster(ctx, c.IdentityDB, c.SchedDB)
	parts := append([]string{"earn: registered"}, steps...)
	parts = append(parts, lines...)
	parts = append(parts, roster)
	if c.SnapshotPath != "" {
		if rows, err := Status(ctx, tc, c.Board); err == nil {
			if err := WriteSnapshot(c.SnapshotPath, rows); err != nil {
				return "", err
			}
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (c *Command) ensureSetup(ctx context.Context, in args) (*client.TorchClient, []string, error) {
	getenv := c.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	var steps []string
	cfgMap, err := onboard.Load(getenv)
	if err != nil {
		return nil, nil, fmt.Errorf("earn: config: %w", err)
	}
	keyFile := cfgMap["ORBIT_AGENT_KEY_FILE"]
	if keyFile == "" {
		keyFile = strings.TrimSpace(getenv("ORBIT_AGENT_KEY_FILE"))
	}
	if !fileExists(keyFile) && strings.TrimSpace(getenv("ORBIT_AGENT_KEY")) == "" {
		res, err := c.Init()
		if err != nil {
			return nil, nil, fmt.Errorf("earn: init: %w", err)
		}
		steps = append(steps, fmt.Sprintf("init: agent wallet %s, balance %s SOL", res.Pubkey, client.FormatSOL(res.Balance)))
		cfgMap, err = onboard.Load(getenv)
		if err != nil {
			return nil, nil, fmt.Errorf("earn: config: %w", err)
		}
	}
	if creator := strings.TrimSpace(cfgMap["ORBIT_VAULT_CREATOR"]); creator == "" {
		line, err := c.vaultCreate(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		steps = append(steps, line)
		cfgMap, err = onboard.Load(getenv)
		if err != nil {
			return nil, nil, fmt.Errorf("earn: config: %w", err)
		}
	}
	tc, err := c.client(ctx)
	if err != nil {
		return nil, nil, err
	}
	if line, err := c.vaultLink(ctx, tc, in); err != nil {
		return nil, nil, err
	} else if line != "" {
		steps = append(steps, line)
	}
	if line, err := c.vaultDeposit(ctx, tc, in); err != nil {
		return nil, nil, err
	} else if line != "" {
		steps = append(steps, line)
	}
	return tc, steps, nil
}

func (c *Command) vaultCreate(ctx context.Context, in args) (string, error) {
	key, err := c.operatorKey(in)
	if err != nil {
		return "", err
	}
	getenv := c.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg, err := client.LoadOperatorCreateConfig(getenv)
	if err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	creator := key.PublicBase58()
	cfg.VaultCreator = creator
	clientNew := c.NewClient
	if clientNew == nil {
		clientNew = client.New
	}
	tc, err := clientNew(cfg)
	if err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	cfgPath, err := onboard.ConfigPath(getenv)
	if err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	recordCreator := func() error {
		return onboard.WriteConfigValue(cfgPath, "ORBIT_VAULT_CREATOR", creator)
	}
	vault := client.TorchVaultPDA(tc.ProgramID, creator)
	info, err := tc.RPC.GetAccountInfo(ctx, vault)
	if err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	if info.Exists {
		if err := recordCreator(); err != nil {
			return "", fmt.Errorf("earn: vault create: %w", err)
		}
		return fmt.Sprintf("vault: exists %s (recorded %s)", vault, creator), nil
	}
	ix, err := client.VaultCreateIx(tc.ProgramID, creator, tc.IDL)
	if err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	sig, err := client.SendVaultIx(ctx, tc, key, "create_vault", []sol.Instruction{ix})
	if err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	if err := recordCreator(); err != nil {
		return "", fmt.Errorf("earn: vault create: %w", err)
	}
	return fmt.Sprintf("vault: created %s (%s)", vault, sig), nil
}

func (c *Command) vaultLink(ctx context.Context, tc *client.TorchClient, in args) (string, error) {
	hot := tc.AgentPublic()
	info, err := tc.RPC.GetAccountInfo(ctx, client.VaultWalletLinkPDA(tc.ProgramID, hot))
	if err != nil {
		return "", fmt.Errorf("earn: vault link: %w", err)
	}
	if info.Exists {
		return "", nil
	}
	key, err := c.operatorKey(in)
	if err != nil {
		return "", err
	}
	ix, err := client.VaultLinkIx(tc.ProgramID, key.PublicBase58(), hot, tc.IDL)
	if err != nil {
		return "", fmt.Errorf("earn: vault link: %w", err)
	}
	sig, err := client.SendVaultIx(ctx, tc, key, "link_wallet", []sol.Instruction{ix})
	if err != nil {
		return "", fmt.Errorf("earn: vault link: %w", err)
	}
	if err := c.confirmVaultTx(ctx, tc, "link_wallet", sig); err != nil {
		return "", err
	}
	return fmt.Sprintf("vault: linked %s (%s)", hot, sig), nil
}

func (c *Command) vaultDeposit(ctx context.Context, tc *client.TorchClient, in args) (string, error) {
	getenv := c.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfgMap, err := onboard.Load(getenv)
	if err != nil {
		return "", fmt.Errorf("earn: vault deposit: %w", err)
	}
	if strings.TrimSpace(cfgMap["ORBIT_VAULT_DEPOSITED"]) != "" {
		return "", nil
	}
	// The deposit gate is the recorded setup marker or the vault record's
	// total_deposited, never the running balance (which the board spends).
	info, err := tc.RPC.GetAccountInfo(ctx, client.TorchVaultPDA(tc.ProgramID, tc.VaultCreator))
	if err != nil {
		return "", fmt.Errorf("earn: vault deposit: %w", err)
	}
	if info.Exists {
		rec, err := client.DecodeTorchVault(info.Data)
		if err != nil {
			return "", fmt.Errorf("earn: vault deposit: vault record: %w", err)
		}
		if rec.TotalDeposited > 0 {
			if err := c.recordDeposited(getenv); err != nil {
				return "", fmt.Errorf("earn: vault deposit: %w", err)
			}
			return "", nil
		}
	}
	oneSOL, err := client.ParseSOLAmount("1")
	if err != nil {
		return "", fmt.Errorf("earn: vault deposit: %w", err)
	}
	key, err := c.operatorKey(in)
	if err != nil {
		return "", err
	}
	ix, err := client.VaultDepositIx(tc.ProgramID, key.PublicBase58(), oneSOL, tc.IDL)
	if err != nil {
		return "", fmt.Errorf("earn: vault deposit: %w", err)
	}
	sig, err := client.SendVaultIx(ctx, tc, key, "deposit_vault", []sol.Instruction{ix})
	if err != nil {
		return "", fmt.Errorf("earn: vault deposit: %w", err)
	}
	if err := c.confirmVaultTx(ctx, tc, "deposit_vault", sig); err != nil {
		return "", err
	}
	if err := c.recordDeposited(getenv); err != nil {
		return "", fmt.Errorf("earn: vault deposit: %w", err)
	}
	return fmt.Sprintf("vault: deposited 1 SOL (%s)", sig), nil
}

func (c *Command) recordDeposited(getenv func(string) string) error {
	cfgPath, err := onboard.ConfigPath(getenv)
	if err != nil {
		return err
	}
	return onboard.WriteConfigValue(cfgPath, "ORBIT_VAULT_DEPOSITED", "1")
}

func (c *Command) confirmVaultTx(ctx context.Context, tc *client.TorchClient, kind, sig string) error {
	if err := client.WaitConfirmed(ctx, tc.RPC, sig, 30*time.Second); err != nil {
		return fmt.Errorf("earn: vault %s: %w", kind, err)
	}
	return nil
}

func (c *Command) operatorKey(in args) (sol.Keypair, error) {
	op := c.Operator
	if op == nil {
		return sol.Keypair{}, fmt.Errorf("earn: operator key required (named at the call, never stored): -operator-key, -operator-key-path, or ORBIT_OPERATOR_KEY(_PATH)")
	}
	return op(in.operatorKey, in.operatorKeyPath)
}

func (c *Command) client(ctx context.Context) (*client.TorchClient, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("earn: no client seam")
	}
	tc, err := c.Client()
	if err != nil {
		return nil, fmt.Errorf("earn: no orbit config (run /earn): %w", err)
	}
	return tc, nil
}

func (c *Command) runner() string {
	if c.Self == "" {
		return "orbit run-job"
	}
	return agent.RunnerCommand(c.Self)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

type args struct {
	action          string
	roles           []identity.Role
	goal            string
	operatorKey     string
	operatorKeyPath string
	model           string
}

func parseArgs(s string) (args, error) {
	tokens, err := tokenize(s)
	if err != nil {
		return args{}, err
	}
	out := args{}
	seen := map[identity.Role]bool{}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok {
		case "status", "stop", "start":
			if i != 0 || len(tokens) != 1 {
				return args{}, fmt.Errorf("earn: %s takes no arguments", tok)
			}
			out.action = tok
			continue
		case "--goal":
			v, ok := valueAt(tokens, i+1)
			if !ok {
				return args{}, fmt.Errorf("earn: --goal needs a value")
			}
			out.goal = v
			i++
		case "--operator-key":
			v, ok := valueAt(tokens, i+1)
			if !ok {
				return args{}, fmt.Errorf("earn: --operator-key needs a value")
			}
			out.operatorKey = v
			i++
		case "--operator-key-path":
			v, ok := valueAt(tokens, i+1)
			if !ok {
				return args{}, fmt.Errorf("earn: --operator-key-path needs a value")
			}
			out.operatorKeyPath = v
			i++
		case "--model":
			v, ok := valueAt(tokens, i+1)
			if !ok {
				return args{}, fmt.Errorf("earn: --model needs a value")
			}
			out.model = v
			i++
		default:
			role, err := identity.ParseRole(tok)
			if err != nil {
				return args{}, err
			}
			if seen[role] {
				return args{}, fmt.Errorf("earn: duplicate role %s", role)
			}
			seen[role] = true
			out.roles = append(out.roles, role)
		}
	}
	return out, nil
}

func valueAt(tokens []string, i int) (string, bool) {
	if i >= len(tokens) {
		return "", false
	}
	return tokens[i], true
}

func tokenize(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	quoted := false
	started := false
	flush := func() {
		if started {
			out = append(out, cur.String())
			cur.Reset()
			started = false
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			started = true
		case (r == ' ' || r == '\t') && !quoted:
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("earn: unterminated quote")
	}
	flush()
	return out, nil
}

func Roster(ctx context.Context, db store.DB, sdb sched.DB) string {
	rows, err := identity.List(ctx, db)
	if err != nil || len(rows) == 0 {
		return "earn: roster: no identity rows (register first)"
	}
	var b strings.Builder
	b.WriteString("ROSTER\n")
	b.WriteString(fmt.Sprintf("%-24s %-10s %-8s %-12s %-8s %-7s %s\n", "ID", "ROLE", "MODEL", "CADENCE", "BLOCK", "BUDGET", "JOB"))
	for _, row := range rows {
		jobID := findJobID(ctx, sdb, agent.JobName(row.ID))
		state := "-"
		if jobID != "" {
			state = jobState(ctx, sdb, jobID)
		}
		b.WriteString(fmt.Sprintf("%-24s %-10s %-8s %-12s %-8s $%-6.2f %s\n",
			row.ID, row.Role, row.Model, row.Cadence, row.BlockSize, row.Budget, state))
	}
	return b.String()
}

func findJobID(ctx context.Context, db sched.DB, name string) string {
	var id string
	err := db.DB.QueryRowContext(ctx, `SELECT id FROM jobs WHERE name = ? ORDER BY rowid DESC LIMIT 1`, name).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func jobState(ctx context.Context, db sched.DB, id string) string {
	bound, tx, err := db.TxReadOnly(ctx)
	if err != nil {
		return "?"
	}
	job, err := scheddomain.NewJobDomain().GetJob(bound, id).Row()
	tx.Rollback()
	if err != nil || job == nil {
		return "?"
	}
	return job.State
}

// JobCwd pins the agent jobs to the orbit home: a job's cwd must not
// follow the TUI's cwd (the sessions and caches live under the home).
func (c *Command) JobCwd() (string, error) {
	return client.Home(c.Getenv)
}
