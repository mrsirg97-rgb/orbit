package earn

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/onboard"
	"github.com/mrsirg97-rgb/orbit/sol"
)

type fakeRPC struct {
	accounts map[string]client.AccountInfo
	balance  uint64
	calls    int
}

func (f *fakeRPC) GetLatestBlockhash(ctx context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}
func (f *fakeRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	return "sigEARN1234567890", nil
}
func (f *fakeRPC) GetAccountInfo(ctx context.Context, pubkey string) (client.AccountInfo, error) {
	f.calls++
	return f.accounts[pubkey], nil
}
func (f *fakeRPC) GetTokenAccountsByOwner(ctx context.Context, owner, programID string) ([]client.TokenAccount, error) {
	f.calls++
	return nil, nil
}
func (f *fakeRPC) GetBalance(ctx context.Context, pubkey string) (uint64, error) {
	return f.balance, nil
}
func (f *fakeRPC) GetSignatureStatus(ctx context.Context, signature string) (client.SignatureStatus, error) {
	return client.SignatureStatus{Exists: true, Confirmed: true}, nil
}
func (f *fakeRPC) RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error) {
	return "airdrop-sig", nil
}
func (f *fakeRPC) GetSignaturesForAddress(ctx context.Context, address string, limit int) ([]client.SignatureInfo, error) {
	return nil, nil
}
func (f *fakeRPC) GetTransaction(ctx context.Context, signature string) (*client.Transaction, error) {
	return nil, nil
}

type stubAPI struct {
	pnl client.PnlSummary
}

func (s *stubAPI) Markets(ctx context.Context, q url.Values) ([]client.MarketRow, error) {
	return nil, nil
}
func (s *stubAPI) Market(ctx context.Context, mint string) (client.MarketDetail, error) {
	return client.MarketDetail{}, nil
}
func (s *stubAPI) Messages(ctx context.Context, q url.Values) ([]client.MessageRow, error) {
	return nil, nil
}
func (s *stubAPI) Trades(ctx context.Context, q url.Values) ([]client.TradeRow, error) {
	return nil, nil
}
func (s *stubAPI) Positions(ctx context.Context, q url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (s *stubAPI) PositionEvents(ctx context.Context, q url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (s *stubAPI) Migrations(ctx context.Context, q url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (s *stubAPI) Pnl(ctx context.Context, wallet, vault string) (client.PnlSummary, error) {
	return s.pnl, nil
}
func (s *stubAPI) Swaps(ctx context.Context, q url.Values) ([]client.SwapRow, error) {
	return nil, nil
}

func testClient(t *testing.T, rpc *fakeRPC) *client.TorchClient {
	t.Helper()
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return &client.TorchClient{
		Config: client.Config{
			Indexer:      "https://indexer.example",
			RPC:          "https://rpc.example",
			ProgramID:    client.DevnetProgramID,
			VaultCreator: "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy",
			AgentKey:     kp,
		},
		API: &stubAPI{pnl: client.PnlSummary{TotalRealizedPnl: 2_500_000}},
		RPC: rpc,
	}
}

type fakeCrontab struct {
	text string
}

func (f *fakeCrontab) List() (string, error) { return f.text, nil }
func (f *fakeCrontab) Install(text string) error {
	f.text = text
	return nil
}

type earnHarness struct {
	cmd *Command
	idb store.DB
	sdb sched.DB
}

func newHarness(t *testing.T, getenv func(string) string) *earnHarness {
	t.Helper()
	dir := t.TempDir()
	rpc := &fakeRPC{}
	tc := testClient(t, rpc)
	rpc.accounts = map[string]client.AccountInfo{
		client.TorchVaultPDA(tc.ProgramID, tc.VaultCreator):       {Exists: true, Lamports: 1_000_000_000},
		client.VaultWalletLinkPDA(tc.ProgramID, tc.AgentPublic()): {Exists: true, Lamports: 1_000_000_000},
		client.VaultSolPDA(tc.ProgramID, tc.VaultCreator):         {Exists: true, Lamports: 2_000_000_000 + client.RentExemptZeroData},
	}
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte(sol.Encode(tc.AgentKey.Secret)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "config")
	cfg := "ORBIT_INDEXER=https://indexer.example\n" +
		"ORBIT_RPC=https://rpc.example\n" +
		"ORBIT_PROGRAM_ID=" + client.DevnetProgramID + "\n" +
		"ORBIT_VAULT_CREATOR=8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy\n" +
		"ORBIT_AGENT_KEY_FILE=" + keyFile + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if getenv == nil {
		getenv = func(k string) string {
			if k == "ORBIT_CONFIG" {
				return cfgFile
			}
			return ""
		}
	}
	idb, err := identity.Store(filepath.Join(dir, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	sdb, _, _, err := store.Open(filepath.Join(dir, "scheduler.sqlite"), sched.Statements(), sched.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := board.Open(filepath.Join(dir, "board.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := &Command{
		Getenv: getenv,
		Client: func() (*client.TorchClient, error) { return tc, nil },
		Init: func() (onboard.InitResult, error) {
			return onboard.InitResult{Pubkey: tc.AgentPublic(), Balance: 1_000_000_000}, nil
		},
		Operator:   func(flagKey, flagPath string) (sol.Keypair, error) { return sol.GenerateKeypair() },
		IdentityDB: idb,
		SchedDB:    sdb,
		Crontab:    &fakeCrontab{text: "SHELL=/bin/bash\n"},
		Board:      &board.Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: bs},
		Self:       "/x/orbit",
		Cwd:        dir,
		Session:    "earn-test",
		Model:      func() string { return "dsv4" },
	}
	return &earnHarness{cmd: cmd, idb: idb, sdb: sdb}
}

func TestParseArgs(t *testing.T) {
	in, err := parseArgs(`architect worker --goal "Research briefs." --operator-key-path /k --model dsv4`)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.roles) != 2 || in.goal != "Research briefs." || in.operatorKeyPath != "/k" || in.model != "dsv4" {
		t.Errorf("args: %+v", in)
	}
	for _, tok := range []string{"status", "stop", "start"} {
		got, err := parseArgs(tok)
		if err != nil || got.action != tok {
			t.Errorf("parseArgs(%s): %+v %v", tok, got, err)
		}
	}
	if _, err := parseArgs("worker worker"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate role: %v", err)
	}
	if _, err := parseArgs(`--goal "unterminated`); err == nil || !strings.Contains(err.Error(), "quote") {
		t.Errorf("unterminated quote: %v", err)
	}
}

func TestRegisterAsksForRolesAndGoal(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	if _, err := h.cmd.Run(context.Background(), "", nil); err == nil || !strings.Contains(err.Error(), "which roles") {
		t.Errorf("missing roles: %v", err)
	}
	if _, err := h.cmd.Run(context.Background(), "architect", nil); err == nil || !strings.Contains(err.Error(), "goal") {
		t.Errorf("architect without goal: %v", err)
	}
}

func TestRegisterWizardRegistersRolesAndJobs(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	out, err := h.cmd.Run(context.Background(), `architect worker --goal "Research briefs."`, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"earn: registered", "-architect", "-worker", "ROSTER", "dsv4", "0 12 * * *", "0 */2 * * *"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	rows, err := identity.List(context.Background(), h.idb)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("identity rows: %d, want 2", len(rows))
	}
}

func TestRegisterWizardRunsInitWhenKeyMissing(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, func(k string) string {
		if k == "ORBIT_CONFIG" {
			return filepath.Join(dir, "config")
		}
		return ""
	})
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	cfg := "ORBIT_INDEXER=https://indexer.example\nORBIT_RPC=https://rpc.example\nORBIT_PROGRAM_ID=" + client.DevnetProgramID + "\nORBIT_VAULT_CREATOR=8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy\nORBIT_AGENT_KEY_FILE=" + filepath.Join(dir, "missing-key") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	initCalled := false
	h.cmd.Init = func() (onboard.InitResult, error) {
		initCalled = true
		return onboard.InitResult{Pubkey: "hot", Balance: 1}, nil
	}
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatal(err)
	}
	if !initCalled {
		t.Error("init was not called for a missing hot key")
	}
}

func TestStopAndStart(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatal(err)
	}
	out, err := h.cmd.Run(context.Background(), "stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "paused") {
		t.Errorf("stop output: %s", out)
	}
	out, err = h.cmd.Run(context.Background(), "start", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "resumed") {
		t.Errorf("start output: %s", out)
	}
}

func TestStatusRows(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	out, err := h.cmd.Run(context.Background(), "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("status lines: %d, want 4:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "projects held:") || !strings.HasPrefix(lines[3], "PnL since start: +0.0025 SOL") {
		t.Errorf("status rows:\n%s", out)
	}
}

func TestSnapshotLocalRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	if _, ok, err := Snapshot(path); err != nil || ok {
		t.Fatalf("missing snapshot: ok=%v err=%v", ok, err)
	}
	want := Rows{Held: 3, OpenClaims: 1, LastMemo: "just now · \"backed\"", PnLSOL: 2.5}
	if err := WriteSnapshot(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Snapshot(path)
	if err != nil || !ok {
		t.Fatalf("snapshot read: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("snapshot rows: %+v, want %+v", got, want)
	}
}

func TestRowsFromBriefNoChainCalls(t *testing.T) {
	dir := t.TempDir()
	rpc := &fakeRPC{}
	tc := testClient(t, rpc)
	rpc.accounts = map[string]client.AccountInfo{
		client.TorchVaultPDA(tc.ProgramID, tc.VaultCreator):       {Exists: true, Lamports: 1_000_000_000},
		client.VaultWalletLinkPDA(tc.ProgramID, tc.AgentPublic()): {Exists: true, Lamports: 1_000_000_000},
		client.VaultSolPDA(tc.ProgramID, tc.VaultCreator):         {Exists: true, Lamports: 2_000_000_000 + client.RentExemptZeroData},
	}
	bs, err := board.Open(filepath.Join(dir, "board.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer bs.DB.Close()
	st := &board.Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: bs}
	read := brief.ReadState{
		PnL: brief.PnlSummary{TotalRealizedPnl: 5_000_000},
		Holdings: []brief.Holding{
			{Mint: "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", Raw: 123},
			{Mint: "6wDUn9V7fuP1Ujn6o3xk4yFh4EE3F65LpgQsrNjTjmVx", Raw: 456},
		},
	}
	rows, err := RowsFromBrief(context.Background(), read, tc, st)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Held != 2 || rows.PnLSOL != 0.005 {
		t.Errorf("rows: %+v", rows)
	}
	if rpc.calls != 0 {
		t.Errorf("RowsFromBrief hit the RPC %d times", rpc.calls)
	}
}
