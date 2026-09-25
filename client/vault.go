package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/sol"
)

// ── vault instruction builders (operator/admin path) ──────────────────

// VaultCreateIx is create_vault for the creator (the operator key signs).
func VaultCreateIx(programID, creator string, idl *idl.IDL) (sol.Instruction, error) {
	disc, err := idl.Discriminator("create_vault")
	if err != nil {
		return sol.Instruction{}, err
	}
	return sol.Instruction{
		ProgramID: programID,
		Accounts: []sol.AccountMeta{
			{Pubkey: creator, IsSigner: true, IsWritable: true},
			{Pubkey: TorchVaultPDA(programID, creator), IsWritable: true},
			{Pubkey: VaultSolPDA(programID, creator), IsWritable: true},
			{Pubkey: VaultWalletLinkPDA(programID, creator), IsWritable: true},
			{Pubkey: SystemProgram},
		},
		Data: disc,
	}, nil
}

// VaultDepositIx is deposit_vault (anyone can deposit; the payer signs).
func VaultDepositIx(programID, depositor string, solAmount uint64, idl *idl.IDL) (sol.Instruction, error) {
	disc, err := idl.Discriminator("deposit_vault")
	if err != nil {
		return sol.Instruction{}, err
	}
	args, err := idl.BorshArgs("deposit_vault", map[string]any{"sol_amount": solAmount})
	if err != nil {
		return sol.Instruction{}, err
	}
	return sol.Instruction{
		ProgramID: programID,
		Accounts: []sol.AccountMeta{
			{Pubkey: depositor, IsSigner: true, IsWritable: true},
			{Pubkey: TorchVaultPDA(programID, depositor), IsWritable: true},
			{Pubkey: VaultSolPDA(programID, depositor), IsWritable: true},
			{Pubkey: SystemProgram},
		},
		Data: append(append([]byte{}, disc...), args...),
	}, nil
}

// VaultWithdrawIx is withdraw_vault (authority only).
func VaultWithdrawIx(programID, authority string, solAmount uint64, idl *idl.IDL) (sol.Instruction, error) {
	disc, err := idl.Discriminator("withdraw_vault")
	if err != nil {
		return sol.Instruction{}, err
	}
	args, err := idl.BorshArgs("withdraw_vault", map[string]any{"sol_amount": solAmount})
	if err != nil {
		return sol.Instruction{}, err
	}
	return sol.Instruction{
		ProgramID: programID,
		Accounts: []sol.AccountMeta{
			{Pubkey: authority, IsSigner: true, IsWritable: true},
			{Pubkey: TorchVaultPDA(programID, authority), IsWritable: true},
			{Pubkey: VaultSolPDA(programID, authority), IsWritable: true},
			{Pubkey: SystemProgram},
		},
		Data: append(append([]byte{}, disc...), args...),
	}, nil
}

// VaultLinkIx is link_wallet: the authority links another wallet into the
// vault (the wallet's own [vault_wallet, wallet] PDA is initialized).
func VaultLinkIx(programID, authority, wallet string, idl *idl.IDL) (sol.Instruction, error) {
	disc, err := idl.Discriminator("link_wallet")
	if err != nil {
		return sol.Instruction{}, err
	}
	return sol.Instruction{
		ProgramID: programID,
		Accounts: []sol.AccountMeta{
			{Pubkey: authority, IsSigner: true, IsWritable: true},
			{Pubkey: TorchVaultPDA(programID, authority), IsWritable: true},
			{Pubkey: wallet},
			{Pubkey: VaultWalletLinkPDA(programID, wallet), IsWritable: true},
			{Pubkey: SystemProgram},
		},
		Data: disc,
	}, nil
}

// VaultUnlinkIx is unlink_wallet: the authority removes a wallet's link.
func VaultUnlinkIx(programID, authority, wallet string, idl *idl.IDL) (sol.Instruction, error) {
	disc, err := idl.Discriminator("unlink_wallet")
	if err != nil {
		return sol.Instruction{}, err
	}
	return sol.Instruction{
		ProgramID: programID,
		Accounts: []sol.AccountMeta{
			{Pubkey: authority, IsSigner: true, IsWritable: true},
			{Pubkey: TorchVaultPDA(programID, authority), IsWritable: true},
			{Pubkey: wallet},
			{Pubkey: VaultWalletLinkPDA(programID, wallet), IsWritable: true},
			{Pubkey: SystemProgram},
		},
		Data: disc,
	}, nil
}

// ── operator send ─────────────────────────────────────────────────────

// SendVaultIx compiles, signs with the operator key, and sends one vault
// instruction. The devnet gate stays: a non-devnet program refuses.
func SendVaultIx(ctx context.Context, tc *TorchClient, key sol.Keypair, kind string, ixs []sol.Instruction) (string, error) {
	if !tc.AllowWrite {
		return "", fmt.Errorf("vault %s: write disabled (devnet gate off)", kind)
	}
	blockhash, err := tc.RPC.GetLatestBlockhash(ctx)
	if err != nil {
		return "", fmt.Errorf("vault %s: %w", kind, err)
	}
	msg, err := sol.Compile(blockhash, key.PublicBase58(), ixs)
	if err != nil {
		return "", fmt.Errorf("vault %s: %w", kind, err)
	}
	signed := sol.SignVersionedTx(msg, key)
	sig, err := tc.RPC.SendTransaction(ctx, signed)
	if err != nil {
		return "", fmt.Errorf("vault %s: %w", kind, err)
	}
	return sig, nil
}

// ── show (read the vault from the chain) ──────────────────────────────

// TorchVaultRecord is the decoded TorchVault account (state.rs).
type TorchVaultRecord struct {
	Creator        string
	Authority      string
	TotalDeposited uint64
	TotalWithdrawn uint64
	TotalSpent     uint64
	TotalReceived  uint64
	LinkedWallets  uint8
	CreatedAt      int64
	Bump           uint8
}

// DecodeTorchVault decodes the 114-byte TorchVault account (discriminator +
// creator + authority + four u64 totals + linked_wallets + created_at +
// bump).
func DecodeTorchVault(data []byte) (TorchVaultRecord, error) {
	if len(data) < 114 {
		return TorchVaultRecord{}, fmt.Errorf("vault: account data %d bytes, want >= 114", len(data))
	}
	var r TorchVaultRecord
	r.Creator = sol.Encode(data[8:40])
	r.Authority = sol.Encode(data[40:72])
	r.TotalDeposited = binary.LittleEndian.Uint64(data[72:80])
	r.TotalWithdrawn = binary.LittleEndian.Uint64(data[80:88])
	r.TotalSpent = binary.LittleEndian.Uint64(data[88:96])
	r.TotalReceived = binary.LittleEndian.Uint64(data[96:104])
	r.LinkedWallets = data[104]
	r.CreatedAt = int64(binary.LittleEndian.Uint64(data[105:113]))
	r.Bump = data[113]
	return r, nil
}

// VaultShow is the read behind `orbit vault show`: the vault account, its
// System-owned SOL home, and the linked-wallet count, all from the chain.
type VaultShow struct {
	Vault          string
	Creator        string
	Authority      string
	VaultSOL       uint64
	LinkedWallets  uint8
	TotalDeposited uint64
	TotalWithdrawn uint64
	TotalSpent     uint64
	TotalReceived  uint64
	CreatedAt      int64
}

// ShowVault fetches and decodes the creator's vault.
func ShowVault(ctx context.Context, tc *TorchClient, creator string) (VaultShow, error) {
	vault := TorchVaultPDA(tc.ProgramID, creator)
	acct, err := tc.RPC.GetAccountInfo(ctx, vault)
	if err != nil {
		return VaultShow{}, fmt.Errorf("vault show: %w", err)
	}
	if !acct.Exists {
		return VaultShow{}, fmt.Errorf("vault show: %s has no vault (run 'orbit vault create')", creator)
	}
	rec, err := DecodeTorchVault(acct.Data)
	if err != nil {
		return VaultShow{}, err
	}
	vsol, err := tc.RPC.GetAccountInfo(ctx, VaultSolPDA(tc.ProgramID, creator))
	if err != nil {
		return VaultShow{}, fmt.Errorf("vault show: vault sol: %w", err)
	}
	var vsolBalance uint64
	if vsol.Exists {
		if vsol.Lamports > RentExemptZeroData {
			vsolBalance = vsol.Lamports - RentExemptZeroData
		}
	}
	return VaultShow{
		Vault:          vault,
		Creator:        rec.Creator,
		Authority:      rec.Authority,
		VaultSOL:       vsolBalance,
		LinkedWallets:  rec.LinkedWallets,
		TotalDeposited: rec.TotalDeposited,
		TotalWithdrawn: rec.TotalWithdrawn,
		TotalSpent:     rec.TotalSpent,
		TotalReceived:  rec.TotalReceived,
		CreatedAt:      rec.CreatedAt,
	}, nil
}

// FormatSOL renders lamports as a SOL amount string.
func FormatSOL(lamports uint64) string {
	return fmt.Sprintf("%d.%09d", lamports/1_000_000_000, lamports%1_000_000_000)
}

// trimSolAmount parses a SOL amount ("1", "1.5", "0.25") into lamports.
func ParseSOLAmount(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("amount required")
	}
	parts := strings.SplitN(s, ".", 2)
	if len(parts) > 2 {
		return 0, fmt.Errorf("amount %q: too many dots", s)
	}
	whole, err := parseU64(parts[0])
	if err != nil {
		return 0, fmt.Errorf("amount %q: %w", s, err)
	}
	frac := uint64(0)
	if len(parts) == 2 {
		if len(parts[1]) > 9 {
			return 0, fmt.Errorf("amount %q: more than 9 decimals", s)
		}
		f := parts[1]
		if len(f) > 0 && !allDigits(f) {
			return 0, fmt.Errorf("amount %q: bad decimals", s)
		}
		for len(f) < 9 {
			f += "0"
		}
		frac, err = parseU64(f)
		if err != nil {
			return 0, fmt.Errorf("amount %q: %w", s, err)
		}
	}
	return whole*1_000_000_000 + frac, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
