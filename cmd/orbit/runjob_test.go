package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/sol"
)

type runjobFakeRPC struct {
	accounts map[string]client.AccountInfo
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
func (f *runjobFakeRPC) GetBalance(context.Context, string) (uint64, error) { return 0, nil }
func (f *runjobFakeRPC) GetSignatureStatus(context.Context, string) (client.SignatureStatus, error) {
	return client.SignatureStatus{Exists: true, Confirmed: true}, nil
}
func (f *runjobFakeRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (f *runjobFakeRPC) GetSignaturesForAddress(context.Context, string, int) ([]client.SignatureInfo, error) {
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

func TestFireSandboxAndSwapFromSettingsAndEnv(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "settings.json"),
		[]byte(`{"sandbox": "jailed", "swapUrl": "http://10.0.0.1:9000"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(k string) string {
		switch k {
		case "RIG_HOME":
			return home
		}
		return ""
	}
	sandbox, swapURL, err := fireSandboxSwap(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if sandbox != "jailed" {
		t.Errorf("sandbox %q, want settings.json's jailed", sandbox)
	}
	if swapURL != "http://10.0.0.1:9000" {
		t.Errorf("swap url %q, want settings.json's", swapURL)
	}

	env := func(k string) string {
		switch k {
		case "RIG_HOME":
			return home
		case "ORBIT_SANDBOX":
			return "landlock"
		case "RIG_SWAP_URL":
			return "http://10.0.0.2:9000"
		}
		return ""
	}
	sandbox, swapURL, err = fireSandboxSwap(env)
	if err != nil {
		t.Fatal(err)
	}
	if sandbox != "landlock" {
		t.Errorf("env sandbox %q, want landlock to override settings", sandbox)
	}
	if swapURL != "http://10.0.0.2:9000" {
		t.Errorf("env swap url %q, want the env to override settings", swapURL)
	}
}
