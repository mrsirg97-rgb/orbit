package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/sol"
	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
)

func identityStore() store.DB {
	home, err := rigHome()
	if err != nil {
		die("%v", err)
	}
	dir := filepath.Join(home, "orbit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		die("%v", err)
	}
	db, err := identity.Store(filepath.Join(dir, "identity.sqlite"))
	if err != nil {
		die("identity store: %v", err)
	}
	return db
}

func schedStore() sched.DB {
	home, err := rigHome()
	if err != nil {
		die("%v", err)
	}
	dir := filepath.Join(home, "scheduler")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		die("%v", err)
	}
	db, quarantined, report, err := store.Open(filepath.Join(dir, "global.sqlite"), sched.Statements(), sched.SchemaVersion)
	if err != nil {
		die("scheduler store: %v", err)
	}
	if quarantined != "" {
		die("scheduler store: quarantined %s", quarantined)
	}
	if report != "" {
		die("scheduler store: %s", report)
	}
	return db
}

type UnsignedIx struct {
	ProgramID string            `json:"program_id"`
	Accounts  []sol.AccountMeta `json:"accounts"`
	Data      string            `json:"data"`
}

func bootstrapTx(tc *client.TorchClient, cfg client.Config, name string, args map[string]any) UnsignedIx {
	disc, err := tc.IDL.Discriminator(name)
	if err != nil {
		die("bootstrap: %v", err)
	}
	data := append([]byte{}, disc...)
	var encoded []byte
	if args != nil {
		encoded, err = tc.IDL.BorshArgs(name, args)
		if err != nil {
			die("bootstrap: %v", err)
		}
		data = append(data, encoded...)
	}
	ix := UnsignedIx{ProgramID: cfg.ProgramID, Data: base64.StdEncoding.EncodeToString(data)}
	switch name {
	case "create_vault":
		ix.Accounts = []sol.AccountMeta{
			{Pubkey: cfg.VaultCreator, IsSigner: true, IsWritable: true},
			{Pubkey: client.TorchVaultPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.VaultSolPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.VaultWalletLinkPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.SystemProgram},
		}
	case "link_wallet":
		ix.Accounts = []sol.AccountMeta{
			{Pubkey: cfg.VaultCreator, IsSigner: true, IsWritable: true},
			{Pubkey: client.TorchVaultPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: tc.AgentPublic()},
			{Pubkey: client.VaultWalletLinkPDA(cfg.ProgramID, tc.AgentPublic()), IsWritable: true},
			{Pubkey: client.SystemProgram},
		}
	case "deposit_vault":
		ix.Accounts = []sol.AccountMeta{
			{Pubkey: cfg.VaultCreator, IsSigner: true, IsWritable: true},
			{Pubkey: client.TorchVaultPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.VaultSolPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.SystemProgram},
		}
	default:
		die("bootstrap: unknown instruction %q", name)
	}
	return ix
}
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
