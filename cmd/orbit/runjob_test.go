package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/earn"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/sol"
)

type runjobFakeRPC struct {
	accounts map[string]client.AccountInfo
	balance  uint64
}

func (f *runjobFakeRPC) GetLatestBlockhash(context.Context) (string, error) {
	return "11111111111111111111111111111111", nil
}
func (f *runjobFakeRPC) SendTransaction(context.Context, []byte) (string, error) { return "", nil }
func (f *runjobFakeRPC) GetAccountInfo(ctx context.Context, pubkey string) (client.AccountInfo, error) {
	return f.accounts[pubkey], nil
}
func (f *runjobFakeRPC) GetTokenAccountsByOwner(context.Context, string, string) ([]client.TokenAccount, error) {
	return nil, nil
}
func (f *runjobFakeRPC) GetBalance(context.Context, string) (uint64, error) { return f.balance, nil }
func (f *runjobFakeRPC) GetSignatureStatus(context.Context, string) (client.SignatureStatus, error) {
	return client.SignatureStatus{Exists: true, Confirmed: true}, nil
}
func (f *runjobFakeRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (f *runjobFakeRPC) GetSignaturesForAddress(context.Context, string, int, string) ([]client.SignatureInfo, error) {
	return nil, nil
}
func (f *runjobFakeRPC) GetTransaction(context.Context, string) (*client.Transaction, error) {
	return nil, nil
}

type runjobStubAPI struct{}

func (runjobStubAPI) Markets(context.Context, url.Values) ([]client.MarketRow, error) {
	return nil, nil
}
func (runjobStubAPI) Market(context.Context, string) (client.MarketDetail, error) {
	return client.MarketDetail{}, nil
}
func (runjobStubAPI) Messages(context.Context, url.Values) ([]client.MessageRow, error) {
	return nil, nil
}
func (runjobStubAPI) Trades(context.Context, url.Values) ([]client.TradeRow, error) { return nil, nil }
func (runjobStubAPI) Positions(context.Context, url.Values) ([]client.PositionRow, error) {
	return nil, nil
}
func (runjobStubAPI) PositionEvents(context.Context, url.Values) ([]client.PositionEventRow, error) {
	return nil, nil
}
func (runjobStubAPI) Migrations(context.Context, url.Values) ([]client.MigrationRow, error) {
	return nil, nil
}
func (runjobStubAPI) Pnl(context.Context, string, string) (client.PnlSummary, error) {
	return client.PnlSummary{}, nil
}
func (runjobStubAPI) Swaps(context.Context, url.Values) ([]client.SwapRow, error) { return nil, nil }

func runjobClient(t *testing.T) *client.TorchClient {
	t.Helper()
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	const creator = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	rpc := &runjobFakeRPC{accounts: map[string]client.AccountInfo{
		client.TorchVaultPDA(client.DevnetProgramID, creator): {Exists: true, Lamports: 1_000_000_000},
		client.VaultSolPDA(client.DevnetProgramID, creator): {
			Exists: true, Lamports: 1_000_000_000 + client.RentExemptZeroData,
		},
	}}
	return &client.TorchClient{
		Config: client.Config{
			ProgramID: client.DevnetProgramID, VaultCreator: creator, AgentKey: kp,
		},
		API: runjobStubAPI{},
		RPC: rpc,
	}
}

func TestFireRefusesWhenHotWalletEmpty(t *testing.T) {
	tc := runjobClient(t)
	err := checkFireFunded(context.Background(), tc)
	if err == nil {
		t.Fatal("a fire with an empty hot wallet must refuse before spawn")
	}
	msg := err.Error()
	for _, want := range []string{"send devnet SOL or wait for the faucet", "0.005", tc.AgentPublic()} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q: %v", want, msg)
		}
	}
}

func TestFireAboveFloorPassesTheCheck(t *testing.T) {
	tc := runjobClient(t)
	tc.RPC.(*runjobFakeRPC).balance = earn.FundingFloorLamports
	if err := checkFireFunded(context.Background(), tc); err != nil {
		t.Fatalf("a wallet at the floor must fund the fire: %v", err)
	}
}

func TestFireBriefCarriesGoal(t *testing.T) {
	row := identity.Row{Name: "architect", Bio: "Frames the brief.", Goal: "Research whether compact briefs degrade decisions."}
	_, text, err := fireBrief(context.Background(), runjobClient(t), row)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "GOAL: Research whether compact briefs degrade decisions.") {
		t.Errorf("the live brief dropped the architect's goal:\n%s", text)
	}
}

func TestFireSandboxIsAlwaysOn(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "settings.json"),
		[]byte(`{"sandbox": "off", "swapUrl": "http://10.0.0.1:9000"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(k string) string {
		switch k {
		case "RIG_HOME":
			return home
		case "ORBIT_SANDBOX":
			return "off"
		case "RIG_SWAP_URL":
			return "http://10.0.0.2:9000"
		}
		return ""
	}
	sandbox, swapURL, err := fireSandboxSwap(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if sandbox != "jailed" {
		t.Errorf("fire sandbox %q, want jailed (always on, settings and env ignored)", sandbox)
	}
	if swapURL != "http://10.0.0.2:9000" {
		t.Errorf("swap url %q, want the env override", swapURL)
	}
}

func TestFireWorkerResolvesHomeAndPinsTheWire(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips the binary build")
	}
	bin := filepath.Join(t.TempDir(), "orbit")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "settings.json"),
		[]byte(`{"allow": ["bash", "read"], "model": "dsv4"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	models := `[{"id":"dsv4","window":393216,"maxTokens":65536,"reserve":104858,"keepRecent":98304,"role":"interactive","efforts":["low","high","max"]}]`
	if err := os.WriteFile(filepath.Join(home, "models.json"), []byte(models), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".rig-job"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".rig-job", "agent-id"), []byte("@APxxxx-worker"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			_, _ = io.WriteString(w, `{"data":[]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer srv.Close()

	cmd := exec.Command(bin, "-p", "hi", "-base-url", srv.URL, "-model", "dsv4")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "RIG_HOME="+filepath.Join(home, ".rig-job"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fire worker: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "python kernel host") {
		t.Errorf("the fire must not wire the python kernel:\n%s", out)
	}

	mu.Lock()
	body := captured
	mu.Unlock()
	if body == "" {
		t.Fatal("the worker never asked the model")
	}
	var req struct {
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("request: %v\n%s", err, body)
	}
	var names []string
	for _, tool := range req.Tools {
		names = append(names, tool.Function.Name)
	}
	if got, want := strings.Join(names, ","), strings.Join(fireToolNames, ","); got != want {
		t.Errorf("fire wire on the model request: %s, want %s", got, want)
	}
	for _, banned := range []string{"bash", "python", "scheduler", "plugin", "plugins", "sessions", "delegate"} {
		for _, n := range names {
			if n == banned {
				t.Errorf("the fire wire must not name %s: %v", banned, names)
			}
		}
	}
}

func TestFireSandboxIgnoringInteractiveOnStillNamesTheSwap(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "settings.json"),
		[]byte(`{"swapUrl": "http://10.0.0.1:9000"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(k string) string {
		if k == "RIG_HOME" {
			return home
		}
		return ""
	}
	sandbox, swapURL, err := fireSandboxSwap(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if sandbox != "jailed" || swapURL != "http://10.0.0.1:9000" {
		t.Errorf("fire sandbox %q swap %q, want jailed + settings.json's swap url", sandbox, swapURL)
	}
}

func TestFireActSnapshotWritesLastFire(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	idb, err := identity.Store(filepath.Join(dir, "identity.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer idb.DB.Close()
	tc := runjobClient(t)
	row, err := identity.NewRow(tc.AgentPublic(), identity.Worker, identity.Overrides{Name: "worker", Model: "dsv4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.Upsert(ctx, idb, row); err != nil {
		t.Fatal(err)
	}
	bs, err := board.Open(filepath.Join(dir, "board.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer bs.DB.Close()
	st := &board.Store{Client: func() (*client.TorchClient, error) { return tc, nil }, DB: bs}
	path := filepath.Join(dir, "status.json")
	fireActSnapshot(ctx, idb, tc, st, path, row.ID, board.Shape{Verb: "note", ID: 7})
	snap, ok, err := earn.ReadSnapshot(path)
	if err != nil || !ok {
		t.Fatalf("snapshot: ok=%v err=%v", ok, err)
	}
	if snap.LastFire.Role != string(identity.Worker) || snap.LastFire.Verb != "note" || snap.LastFire.Task != 7 {
		t.Errorf("last fire: %+v, want worker note #7", snap.LastFire)
	}
	if snap.LastFire.At == "" {
		t.Error("the last fire must carry its time")
	}
}
