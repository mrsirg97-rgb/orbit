package earn

import (
	"context"
	"encoding/binary"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
	scheddomain "github.com/mrsirg97-rgb/rig/store/scheduler/domain"

	"github.com/mrsirg97-rgb/orbit/agent"
	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/onboard"
	"github.com/mrsirg97-rgb/orbit/sol"
)

type fakeRPC struct {
	accounts map[string]client.AccountInfo
	balance  uint64
	calls    int
	sends    int
}

func (f *fakeRPC) GetLatestBlockhash(ctx context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}
func (f *fakeRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	f.sends++
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
	id, err := idl.LoadIDL()
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
			AllowWrite:   true,
		},
		IDL: id,
		API: &stubAPI{pnl: client.PnlSummary{TotalRealizedPnl: 2_500_000}},
		RPC: rpc,
	}
}

func vaultRecordData(creator string, totalDeposited uint64) []byte {
	data := make([]byte, 114)
	pub, err := sol.Decode(creator)
	if err != nil {
		panic(err)
	}
	copy(data[8:40], pub)
	copy(data[40:72], pub)
	binary.LittleEndian.PutUint64(data[72:80], totalDeposited)
	return data
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
	rpc *fakeRPC
	tc  *client.TorchClient
}

func newHarness(t *testing.T, getenv func(string) string) *earnHarness {
	t.Helper()
	dir := t.TempDir()
	rpc := &fakeRPC{}
	tc := testClient(t, rpc)
	rpc.accounts = map[string]client.AccountInfo{
		client.TorchVaultPDA(tc.ProgramID, tc.VaultCreator): {
			Exists: true, Lamports: 1_000_000_000,
			Data: vaultRecordData(tc.VaultCreator, 1_000_000_000),
		},
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
			if k == "RIG_HOME" {
				return dir
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
		Operator: func(flagKey, flagPath string) (sol.Keypair, error) { return sol.GenerateKeypair() },
		NewClient: func(cfg client.Config) (*client.TorchClient, error) {
			tc, err := client.New(cfg)
			if err != nil {
				return nil, err
			}
			tc.RPC = rpc
			return tc, nil
		},
		IdentityDB: idb,
		SchedDB:    sdb,
		Crontab:    &fakeCrontab{text: "SHELL=/bin/bash\n"},
		Board:      &board.Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: bs},
		Self:       "/x/orbit",
		Cwd:        dir,
		Session:    "earn-test",
		Model:      func() string { return "dsv4" },
	}
	return &earnHarness{cmd: cmd, idb: idb, sdb: sdb, rpc: rpc, tc: tc}
}

type failInstallCrontab struct{}

func (f *failInstallCrontab) List() (string, error) { return "SHELL=/bin/bash\n", nil }
func (f *failInstallCrontab) Install(text string) error {
	return errors.New("crontab: install failed")
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
	if _, err := h.cmd.Run(context.Background(), "", nil); err == nil || !strings.Contains(err.Error(), "which roles") || !strings.Contains(err.Error(), "--goal") {
		t.Errorf("missing roles hint: %v", err)
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

func TestRegisterWizardCreatesVaultWhenCreatorMissing(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	agent, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(sol.Encode(agent.Secret)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "config")
	cfg := "ORBIT_INDEXER=https://indexer.example\n" +
		"ORBIT_RPC=https://rpc.example\n" +
		"ORBIT_PROGRAM_ID=" + client.DevnetProgramID + "\n" +
		"ORBIT_AGENT_KEY_FILE=" + keyFile + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, func(k string) string {
		if k == "ORBIT_CONFIG" {
			return cfgFile
		}
		return ""
	})
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	var operator sol.Keypair
	h.cmd.Operator = func(flagKey, flagPath string) (sol.Keypair, error) {
		operator, err = sol.GenerateKeypair()
		return operator, err
	}
	out, err := h.cmd.Run(context.Background(), "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "vault: created") {
		t.Errorf("wizard output missing the vault step:\n%s", out)
	}
	if h.rpc.sends != 1 {
		t.Errorf("sendTransaction calls: %d, want 1 (create_vault must reach the send)", h.rpc.sends)
	}
	b, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ORBIT_VAULT_CREATOR="+operator.PublicBase58()) {
		t.Errorf("config missing the derived creator:\n%s", b)
	}
}

func TestRegisterWizardRecordsExistingVaultWithoutSending(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	agent, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(sol.Encode(agent.Secret)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "config")
	cfg := "ORBIT_INDEXER=https://indexer.example\n" +
		"ORBIT_RPC=https://rpc.example\n" +
		"ORBIT_PROGRAM_ID=" + client.DevnetProgramID + "\n" +
		"ORBIT_AGENT_KEY_FILE=" + keyFile + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, func(k string) string {
		if k == "ORBIT_CONFIG" {
			return cfgFile
		}
		return ""
	})
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	operator, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	h.cmd.Operator = func(flagKey, flagPath string) (sol.Keypair, error) { return operator, nil }
	// The vault for the derived creator already exists on chain (a previous
	// run sent it but crashed before recording the creator).
	h.rpc.accounts[client.TorchVaultPDA(client.DevnetProgramID, operator.PublicBase58())] = client.AccountInfo{
		Exists: true, Lamports: 1_000_000_000,
		Data: vaultRecordData(operator.PublicBase58(), 0),
	}
	out, err := h.cmd.Run(context.Background(), "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "vault: exists") {
		t.Errorf("wizard output missing the exists step:\n%s", out)
	}
	if h.rpc.sends != 0 {
		t.Errorf("sendTransaction calls: %d, want 0 (the existing vault must not be created again)", h.rpc.sends)
	}
	b, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ORBIT_VAULT_CREATOR="+operator.PublicBase58()) {
		t.Errorf("config missing the recorded creator:\n%s", b)
	}
}

func TestRegisterWizardRerunSendsNothing(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(h.cmd.Getenv("ORBIT_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ORBIT_VAULT_DEPOSITED=1") {
		t.Errorf("setup marker not recorded after the first run:\n%s", b)
	}
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatal(err)
	}
	if h.rpc.sends != 0 {
		t.Errorf("sendTransaction calls: %d, want 0 (a rerun on a set-up home sends nothing)", h.rpc.sends)
	}
}

func TestRegisterWizardExistingSetupNeedsNoOperatorKey(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	h.cmd.Operator = func(flagKey, flagPath string) (sol.Keypair, error) {
		return sol.Keypair{}, errors.New("operator key required: -operator-key, -operator-key-path, or ORBIT_OPERATOR_KEY(_PATH)")
	}
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatalf("a wizard run on an existing setup must not resolve the operator key: %v", err)
	}
	if h.rpc.sends != 0 {
		t.Errorf("sendTransaction calls: %d, want 0 (nothing to sign on an existing setup)", h.rpc.sends)
	}
}

func TestRegisterWizardFailedAfterDepositRerunsWithoutDeposit(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	// The vault exists but has never been deposited: the wizard must send it.
	h.rpc.accounts[client.TorchVaultPDA(h.tc.ProgramID, h.tc.VaultCreator)] = client.AccountInfo{
		Exists: true, Lamports: 1_000_000_000,
		Data: vaultRecordData(h.tc.VaultCreator, 0),
	}
	h.cmd.Crontab = &failInstallCrontab{}
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err == nil || !strings.Contains(err.Error(), "crontab") {
		t.Fatalf("first run must fail at the job after the deposit: %v", err)
	}
	if h.rpc.sends != 1 {
		t.Fatalf("sendTransaction calls: %d, want 1 (the deposit)", h.rpc.sends)
	}
	h.cmd.Crontab = &fakeCrontab{text: "SHELL=/bin/bash\n"}
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatal(err)
	}
	if h.rpc.sends != 1 {
		t.Errorf("sendTransaction calls: %d, want 1 (the rerun must not deposit again)", h.rpc.sends)
	}
}

func TestRegisterWizardAddsRoleToExistingSetup(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err != nil {
		t.Fatal(err)
	}
	out, err := h.cmd.Run(context.Background(), `architect worker --goal "Research briefs."`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "created") || !strings.Contains(out, "updated") {
		t.Errorf("the existing role's job must be refreshed, the new role created:\n%s", out)
	}
	rows, err := identity.List(context.Background(), h.idb)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("identity rows: %d, want 2", len(rows))
	}
	if h.rpc.sends != 0 {
		t.Errorf("sendTransaction calls: %d, want 0", h.rpc.sends)
	}
}

func TestRegisterModelCheckBeforeChainSpend(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	agent, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(sol.Encode(agent.Secret)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "config")
	cfg := "ORBIT_INDEXER=https://indexer.example\n" +
		"ORBIT_RPC=https://rpc.example\n" +
		"ORBIT_PROGRAM_ID=" + client.DevnetProgramID + "\n" +
		"ORBIT_AGENT_KEY_FILE=" + keyFile + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, func(k string) string {
		if k == "ORBIT_CONFIG" {
			return cfgFile
		}
		return ""
	})
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	h.cmd.Model = nil
	if _, err := h.cmd.Run(context.Background(), "worker", nil); err == nil || !strings.Contains(err.Error(), "no model") {
		t.Fatalf("missing model: %v", err)
	}
	if h.rpc.sends != 0 {
		t.Errorf("sendTransaction calls: %d, want 0 (the model check must precede any chain spend)", h.rpc.sends)
	}
	b, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ORBIT_VAULT_CREATOR") {
		t.Errorf("a model-less run must not touch the config:\n%s", b)
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

func TestRegisterPinsJobCwdToOrbitHome(t *testing.T) {
	h := newHarness(t, nil)
	defer h.idb.DB.Close()
	defer h.sdb.DB.Close()
	if _, err := h.cmd.Run(context.Background(), `architect worker --goal "Research briefs."`, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := identity.List(context.Background(), h.idb)
	if err != nil {
		t.Fatal(err)
	}
	jobKey := agent.JobName(rows[0].ID)
	bound, tx, err := h.sdb.TxReadOnly(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var jobID string
	if err := tx.QueryRowContext(bound, `SELECT id FROM jobs WHERE name = ? ORDER BY rowid DESC LIMIT 1`, jobKey).Scan(&jobID); err != nil {
		t.Fatalf("job %s: %v", jobKey, err)
	}
	job, err := scheddomain.NewJobDomain().GetJob(bound, jobID).Row()
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatalf("job %s not found", jobKey)
	}
	home, err := client.Home(h.cmd.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if job.Cwd != home {
		t.Errorf("job cwd %q, want the orbit home %q (not the TUI's cwd)", job.Cwd, home)
	}
}
