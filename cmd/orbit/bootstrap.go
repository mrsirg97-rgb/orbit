package main

import (
	"encoding/base64"
	"os"
	"path/filepath"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/identity"
	"github.com/mrsirg97-rgb/orbit/sol"
	"github.com/mrsirg97-rgb/rig/store"
	sched "github.com/mrsirg97-rgb/rig/store/scheduler"
)

// identityStore is the agent's identity row store.
func identityStore() store.DB {
	dir := filepath.Join(rigHome(), "orbit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		die("%v", err)
	}
	db, err := identity.Store(filepath.Join(dir, "identity.sqlite"))
	if err != nil {
		die("identity store: %v", err)
	}
	return db
}

// schedStore is the scheduler's own store (the job rows live there).
func schedStore() sched.DB {
	dir := filepath.Join(rigHome(), "scheduler")
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

// UnsignedIx is one serialized instruction for the operator to sign with the
// vault authority key. Data is base64 (discriminator + borsh args).
type UnsignedIx struct {
	ProgramID string            `json:"program_id"`
	Accounts  []sol.AccountMeta `json:"accounts"`
	Data      string            `json:"data"`
}

// bootstrapTx builds the unsigned vault-admin instruction. The operator's
// key is never in the process; the JSON is the handoff.
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
			{Pubkey: cfg.ProgramID},
		}
	case "link_wallet":
		ix.Accounts = []sol.AccountMeta{
			{Pubkey: cfg.VaultCreator, IsSigner: true, IsWritable: true},
			{Pubkey: client.TorchVaultPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: tc.AgentPublic()},
			{Pubkey: client.VaultWalletLinkPDA(cfg.ProgramID, tc.AgentPublic()), IsWritable: true},
			{Pubkey: cfg.ProgramID},
		}
	case "deposit_vault":
		ix.Accounts = []sol.AccountMeta{
			{Pubkey: cfg.VaultCreator, IsSigner: true, IsWritable: true},
			{Pubkey: client.TorchVaultPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.VaultSolPDA(cfg.ProgramID, cfg.VaultCreator), IsWritable: true},
			{Pubkey: client.SystemProgram},
			{Pubkey: cfg.ProgramID},
		}
	default:
		die("bootstrap: unknown instruction %q", name)
	}
	return ix
}
