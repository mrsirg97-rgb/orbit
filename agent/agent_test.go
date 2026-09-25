package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
	scheddomain "github.com/mrsirg97-rgb/rig/store/scheduler/domain"
)

type fakeCrontab struct {
	mu   sync.Mutex
	text string
}

func (f *fakeCrontab) List() (string, error) { f.mu.Lock(); defer f.mu.Unlock(); return f.text, nil }
func (f *fakeCrontab) Install(text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.text = text
	return nil
}

type fakeFetch struct{}

func (fakeFetch) fetch(url string) (json.RawMessage, error) {
	if strings.Contains(url, "/v1/models") {
		return json.RawMessage(`{"data":[{"id":"dsv4","meta":{"llamaswap":{"aliases":[]}},"status":{"value":"loaded"}}]}`), nil
	}
	return json.RawMessage(`{"running":[]}`), nil
}

type fakeSpawn struct {
	mu    sync.Mutex
	calls int
	argv  []string
}

func (f *fakeSpawn) spawn(ctx context.Context, argv []string, cwd string, env []string, observe func([]byte)) (sched.SpawnResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.argv = append([]string{}, argv...)
	return sched.SpawnResult{Exit: 0, Stdout: "ok", Stderr: ""}, nil
}

func openSched(t *testing.T, home string) sched.DB {
	t.Helper()
	db, _, _, err := store.Open(filepath.Join(home, "global.sqlite"), sched.Statements(), sched.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFireRebuildsBriefPerFire(t *testing.T) {
	home := t.TempDir()
	db := openSched(t, home)
	defer db.DB.Close()
	ct := &fakeCrontab{text: "SHELL=/bin/bash\n"}

	row := identity.Row{
		ID: "@AP2B3A", Name: "Torch Agent", Wallet: "So11111111111111111111111111111111111111112",
		Bio:     "A torch market agent.",
		Cadence: "0 */8 * * *", Model: "dsv4", BlockSize: "compact",
	}
	stub := brief.StubBrief(brief.Identity{Name: row.Name, Bio: row.Bio})
	if _, err := Register(context.Background(), db, ct, row, stub, "/x/orbit run-job", t.TempDir(), "sess-agent"); err != nil {
		t.Fatal(err)
	}

	// Two fixture snapshots with different values produce different briefs.
	fixture := func(price float64) brief.ReadState {
		return brief.ReadState{
			Identity: brief.Identity{Name: "@AP2B3A", Bio: "b"},
			PnL:      brief.PnlSummary{TotalRealizedPnl: 2_500_000},
			Markets: []brief.MarketView{
				{Mint: "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", Name: "Torch Test", Symbol: "TST", Status: "BONDING", PriceSOL: price, MCAPSOL: price * 1e9, ValueSOL: price * 1e6},
			},
		}
	}
	brief1, err := brief.Build(fixture(0.00015), brief.Compact)
	if err != nil {
		t.Fatal(err)
	}
	brief2, err := brief.Build(fixture(0.00030), brief.Compact)
	if err != nil {
		t.Fatal(err)
	}
	if brief1 == brief2 {
		t.Fatal("fixture briefs must differ")
	}

	spawn := &fakeSpawn{}
	spawnFn := func(ctx context.Context, argv []string, cwd string, env []string, observe func([]byte)) (sched.SpawnResult, error) {
		return spawn.spawn(ctx, argv, cwd, env, observe)
	}
	runJob := func(ctx context.Context) error {
		t.Helper()
		if err := sched.RunJob("j1", sched.RunOpts{
			Home: home, Crontab: ct, Fetch: fakeFetch{}.fetch, Spawn: spawnFn,
			WorkerCmd: []string{"/x/orbit"}, SwapURL: "http://127.0.0.1:8090",
			Now:     func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
			Sandbox: "off",
		}); err != nil {
			return err
		}
		return nil
	}
	promptOf := func() string {
		t.Helper()
		spawn.mu.Lock()
		defer spawn.mu.Unlock()
		for i := 0; i < len(spawn.argv); i++ {
			if spawn.argv[i] == "-p" && i+1 < len(spawn.argv) {
				return spawn.argv[i+1]
			}
		}
		t.Fatal("spawn argv lacks -p")
		return ""
	}

	// Fire 1: brief built from the first snapshot.
	if err := Fire(context.Background(), db, ct, "j1", row.ID, brief1, "/x/orbit run-job", runJob); err != nil {
		t.Fatal(err)
	}
	got1 := promptOf()
	if !strings.Contains(got1, brief1) {
		t.Errorf("fire 1 prompt is stale: %q, want the fresh brief", got1[:20])
	}

	// Fire 2: the same job, a different snapshot -> a different brief.
	if err := Fire(context.Background(), db, ct, "j1", row.ID, brief2, "/x/orbit run-job", runJob); err != nil {
		t.Fatal(err)
	}
	got2 := promptOf()
	if got2 == got1 {
		t.Error("two fires with different snapshots produced the same brief")
	}
	if !strings.Contains(got2, brief2) {
		t.Errorf("fire 2 prompt: got %q, want brief2", got2[:20])
	}
}

func TestAgentJobFiresTheWorldBlock(t *testing.T) {
	home := t.TempDir()
	db := openSched(t, home)
	defer db.DB.Close()
	ct := &fakeCrontab{text: "SHELL=/bin/bash\n"}

	row := identity.Row{
		ID: "@AP2B3A", Name: "Torch Agent", Wallet: "So11111111111111111111111111111111111111112",
		Bio:     "A torch market agent.",
		Cadence: "0 */8 * * *", Model: "dsv4", Stall: 45, Budget: 1.25, Timeout: 60,
	}
	const worldBlock = "LEGEND\n(&) $ \"*\" → BACK — buy a project\nONE ACTION PER FIRE.\n"
	jobCwd := t.TempDir()
	reply, err := Register(context.Background(), db, ct, row, worldBlock, "/x/orbit run-job", jobCwd, "sess-agent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "created j1") {
		t.Fatalf("create reply: %s", reply)
	}

	// The job row carries the identity's cadence/stall/budget/timeout.
	bound, tx, err := db.TxReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	job, err := scheddomain.NewJobDomain().GetJob(bound, "j1").Row()
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("job row missing")
	}
	if job.Cron != row.Cadence {
		t.Errorf("cron %s, want %s", job.Cron, row.Cadence)
	}
	if job.Model != "dsv4" {
		t.Errorf("model %s", job.Model)
	}
	if job.Stall == nil || *job.Stall != 45 {
		t.Errorf("stall %v", job.Stall)
	}
	if job.Budget == nil || *job.Budget != 1.25 {
		t.Errorf("budget %v", job.Budget)
	}
	if job.Timeout == nil || *job.Timeout != 60 {
		t.Errorf("timeout %v", job.Timeout)
	}
	if !strings.Contains(job.Prompt, worldBlock) {
		t.Errorf("prompt lacks the brief")
	}

	// Fire: the fake spawn captures the argv; the worker is `-p <brief>`.
	spawn := &fakeSpawn{}
	spawnFn := func(ctx context.Context, argv []string, cwd string, env []string, observe func([]byte)) (sched.SpawnResult, error) {
		return spawn.spawn(ctx, argv, cwd, env, observe)
	}
	err = sched.RunJob("j1", sched.RunOpts{
		Home:      home,
		Crontab:   ct,
		Fetch:     fakeFetch{}.fetch,
		Spawn:     spawnFn,
		WorkerCmd: []string{"/x/orbit"},
		SwapURL:   "http://127.0.0.1:8090",
		Now:       func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
		Sandbox:   "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spawn.calls != 1 {
		t.Fatalf("spawn calls %d, want 1", spawn.calls)
	}
	argv := spawn.argv
	found := map[string]string{}
	for i := 0; i < len(argv); i++ {
		if i+1 < len(argv) && strings.HasPrefix(argv[i], "-") {
			found[argv[i]] = argv[i+1]
		}
	}
	if found["-p"] == "" || !strings.Contains(found["-p"], worldBlock) {
		t.Errorf("worker prompt missing the brief: %q", found["-p"])
	}
	if !strings.Contains(found["-p"], sched.ReportBack) {
		t.Errorf("prompt lacks the report-back line")
	}
	if found["-model"] != "dsv4" {
		t.Errorf("model %s", found["-model"])
	}
	if found["-session-id"] == "" || found["-base-url"] == "" {
		t.Errorf("worker argv lacks session/base-url: %v", argv)
	}

	// The run recorded ok.
	rows, err := sched.Runs(context.Background(), db, "j1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rows, "ok") {
		t.Errorf("runs: %s", rows)
	}
	_ = os.Getenv
}

func TestDefaultsLandInJob(t *testing.T) {
	home := t.TempDir()
	db := openSched(t, home)
	defer db.DB.Close()
	ct := &fakeCrontab{text: "SHELL=/bin/bash\n"}

	row, err := identity.NewRow("So11111111111111111111111111111111111111112", identity.Worker, identity.Overrides{Model: "dsv4"})
	if err != nil {
		t.Fatal(err)
	}
	stub := brief.StubBrief(brief.Identity{Name: row.Name, Bio: row.Bio})
	if _, err := Register(context.Background(), db, ct, row, stub, "/x/orbit run-job", t.TempDir(), "sess-agent"); err != nil {
		t.Fatal(err)
	}
	bound, tx, err := db.TxReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	job, err := scheddomain.NewJobDomain().GetJob(bound, "j1").Row()
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("job row missing")
	}
	if job.Cron != "0 */2 * * *" {
		t.Errorf("cron %s, want the worker cadence", job.Cron)
	}
	if job.Budget == nil || *job.Budget != 0.5 {
		t.Errorf("budget %v, want 0.5", job.Budget)
	}
	if job.Stall == nil || *job.Stall != 30 {
		t.Errorf("stall %v, want 30", job.Stall)
	}
	if job.Timeout == nil || *job.Timeout != 45 {
		t.Errorf("timeout %v, want 45", job.Timeout)
	}
	if job.Model != "dsv4" {
		t.Errorf("model %s", job.Model)
	}
	for _, want := range []string{
		"NAME: torch worker",
		"BIO:",
	} {
		if !strings.Contains(job.Prompt, want) {
			t.Errorf("job prompt lacks %q", want)
		}
	}
	for _, gone := range []string{"ROLE:", "MEMO SHAPES:", "STAKE:", "VOICE:"} {
		if strings.Contains(job.Prompt, gone) {
			t.Errorf("job prompt still carries %q", gone)
		}
	}
}

func TestOverridesWin(t *testing.T) {
	home := t.TempDir()
	db := openSched(t, home)
	defer db.DB.Close()
	ct := &fakeCrontab{text: "SHELL=/bin/bash\n"}

	row, err := identity.NewRow("So11111111111111111111111111111111111111112", identity.Worker, identity.Overrides{
		Name: "Beta", Cadence: "0 1 * * *", Model: "qwen3.8-27b", Budget: 2.5, Full: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stub := brief.StubBrief(brief.Identity{Name: row.Name, Bio: row.Bio})
	if _, err := Register(context.Background(), db, ct, row, stub, "/x/orbit run-job", t.TempDir(), "sess-agent"); err != nil {
		t.Fatal(err)
	}
	bound, tx, err := db.TxReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	job, err := scheddomain.NewJobDomain().GetJob(bound, "j1").Row()
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("job row missing")
	}
	if job.Cron != "0 1 * * *" {
		t.Errorf("cron %s, want the override", job.Cron)
	}
	if job.Budget == nil || *job.Budget != 2.5 {
		t.Errorf("budget %v, want 2.5", job.Budget)
	}
	if job.Model != "qwen3.8-27b" {
		t.Errorf("model %s, want the override", job.Model)
	}
	if job.Stall == nil || *job.Stall != 30 {
		t.Errorf("stall %v, want the untouched default 30", job.Stall)
	}
	if row.BlockSize != "full" {
		t.Errorf("block size %s, want the full override", row.BlockSize)
	}
	if row.Name != "Beta" {
		t.Errorf("name %s, want the override", row.Name)
	}
}
