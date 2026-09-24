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

func TestAgentJobFiresTheWorldBlock(t *testing.T) {
	home := t.TempDir()
	db := openSched(t, home)
	defer db.DB.Close()
	ct := &fakeCrontab{text: "SHELL=/bin/bash\n"}

	row := identity.Row{
		ID: "@AP2B3A", Name: "Torch Agent", Wallet: "So11111111111111111111111111111111111111112",
		Bio: "A torch market agent.", Personality: "mercenary",
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
		t.Errorf("prompt lacks the world block")
	}

	// Fire: the fake spawn captures the argv; the worker is `-p <world block>`.
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
		t.Errorf("worker prompt missing the world block: %q", found["-p"])
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
