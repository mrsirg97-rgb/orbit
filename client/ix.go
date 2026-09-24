package client

import (
	"errors"
	"strings"

	"github.com/mrsirg97-rgb/orbit/sol"
)

// Memo caps: the curve path fits 500 chars, the DeepPool swap path 280
// (the SDK's constants, keeping the tx under the 1232-byte legacy limit).
const (
	CurveMemoCap = 500
	SwapMemoCap  = 280
)

// BuildMemo is the SPL Memo instruction: UTF-8 bytes, signer account only.
func BuildMemo(signer, memo string) (sol.Instruction, error) {
	memo = strings.TrimSpace(memo)
	if memo == "" {
		return sol.Instruction{}, errors.New("memo: empty")
	}
	return sol.Instruction{
		ProgramID: MemoProgram,
		Accounts:  []sol.AccountMeta{{Pubkey: signer, IsSigner: true}},
		Data:      []byte(memo),
	}, nil
}

// BuildVaultATAIx is createAssociatedTokenAccountIdempotent (data = [1]):
// payer (signer), ata (writable), owner, mint, system, token program.
func BuildVaultATAIx(payer, mint, owner, ata string) sol.Instruction {
	return sol.Instruction{
		ProgramID: ATProgram,
		Accounts: []sol.AccountMeta{
			{Pubkey: payer, IsSigner: true, IsWritable: true},
			{Pubkey: ata, IsWritable: true},
			{Pubkey: owner},
			{Pubkey: mint},
			{Pubkey: SystemProgram},
			{Pubkey: Token2022Program},
		},
		Data: []byte{1},
	}
}

// BuyAccounts holds everything a curve buy instruction needs.
type BuyAccounts struct {
	Mint         string
	Creator      string
	DevWallet    string
	Buyer        string
	VaultCreator string
	ProgramID    string
	WithATA      bool
}

// BuildBuyViaVault builds buy_via_vault (+ optional vault ATA create) from
// the IDL: discriminator, account order, borsh BuyArgs{sol_amount,
// min_tokens_out}.
func BuildBuyViaVault(idlIx []byte, a BuyAccounts, solAmount, minTokensOut uint64, memo string) ([]sol.Instruction, error) {
	bc := BondingCurvePDA(a.ProgramID, a.Mint)
	tokenVault, err := ATA(a.Mint, bc, Token2022Program)
	if err != nil {
		return nil, err
	}
	treasury := TokenTreasuryPDA(a.ProgramID, a.Mint)
	treasuryToken, err := ATA(a.Mint, treasury, Token2022Program)
	if err != nil {
		return nil, err
	}
	up := UserPositionPDA(a.ProgramID, bc, a.Buyer)
	us := UserStatsPDA(a.ProgramID, a.Buyer)
	vault := TorchVaultPDA(a.ProgramID, a.VaultCreator)
	vaultSol := VaultSolPDA(a.ProgramID, a.VaultCreator)
	link := VaultWalletLinkPDA(a.ProgramID, a.Buyer)
	vaultToken, err := ATA(a.Mint, vault, Token2022Program)
	if err != nil {
		return nil, err
	}
	data := append(append([]byte{}, idlIx...), leU64(solAmount)...)
	data = append(data, leU64(minTokensOut)...)
	ix := sol.Instruction{
		ProgramID: a.ProgramID,
		Accounts: []sol.AccountMeta{
			{Pubkey: a.Buyer, IsSigner: true, IsWritable: true},
			{Pubkey: GlobalConfigPDA(a.ProgramID)},
			{Pubkey: a.DevWallet, IsWritable: true},
			{Pubkey: a.Mint, IsWritable: true},
			{Pubkey: bc, IsWritable: true},
			{Pubkey: BondingCurveSolPDA(a.ProgramID, a.Mint), IsWritable: true},
			{Pubkey: tokenVault, IsWritable: true},
			{Pubkey: treasury, IsWritable: true},
			{Pubkey: TreasurySolVaultPDA(a.ProgramID, a.Mint), IsWritable: true},
			{Pubkey: treasuryToken, IsWritable: true},
			{Pubkey: up, IsWritable: true},
			{Pubkey: us, IsWritable: true},
			{Pubkey: ProtocolTreasuryPDA(a.ProgramID), IsWritable: true},
			{Pubkey: a.Creator, IsWritable: true},
			{Pubkey: vault, IsWritable: true},
			{Pubkey: vaultSol, IsWritable: true},
			{Pubkey: link},
			{Pubkey: vaultToken, IsWritable: true},
			{Pubkey: Token2022Program},
			{Pubkey: ATProgram},
			{Pubkey: SystemProgram},
			{Pubkey: TorchEventAuthorityPDA(a.ProgramID)},
			{Pubkey: a.ProgramID},
		},
		Data: data,
	}
	out := []sol.Instruction{}
	if a.WithATA {
		out = append(out, BuildVaultATAIx(a.Buyer, a.Mint, vault, vaultToken))
	}
	out = append(out, ix)
	if memo != "" {
		m, err := BuildMemo(a.Buyer, memo)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// SellAccounts holds everything a curve sell instruction needs.
type SellAccounts struct {
	Mint         string
	Buyer        string
	VaultCreator string
	ProgramID    string
}

// BuildSellViaVault builds sell_via_vault from the IDL: discriminator,
// account order, borsh SellArgs{token_amount, min_sol_out}.
func BuildSellViaVault(idlIx []byte, a SellAccounts, tokenAmount, minSOLOut uint64, memo string) ([]sol.Instruction, error) {
	bc := BondingCurvePDA(a.ProgramID, a.Mint)
	tokenVault, err := ATA(a.Mint, bc, Token2022Program)
	if err != nil {
		return nil, err
	}
	treasury := TokenTreasuryPDA(a.ProgramID, a.Mint)
	up := UserPositionPDA(a.ProgramID, bc, a.Buyer)
	us := UserStatsPDA(a.ProgramID, a.Buyer)
	vault := TorchVaultPDA(a.ProgramID, a.VaultCreator)
	vaultSol := VaultSolPDA(a.ProgramID, a.VaultCreator)
	link := VaultWalletLinkPDA(a.ProgramID, a.Buyer)
	vaultToken, err := ATA(a.Mint, vault, Token2022Program)
	if err != nil {
		return nil, err
	}
	data := append(append([]byte{}, idlIx...), leU64(tokenAmount)...)
	data = append(data, leU64(minSOLOut)...)
	ix := sol.Instruction{
		ProgramID: a.ProgramID,
		Accounts: []sol.AccountMeta{
			{Pubkey: a.Buyer, IsSigner: true, IsWritable: true},
			{Pubkey: a.Mint},
			{Pubkey: bc, IsWritable: true},
			{Pubkey: BondingCurveSolPDA(a.ProgramID, a.Mint), IsWritable: true},
			{Pubkey: tokenVault, IsWritable: true},
			{Pubkey: up},
			{Pubkey: treasury, IsWritable: true},
			{Pubkey: TreasurySolVaultPDA(a.ProgramID, a.Mint), IsWritable: true},
			{Pubkey: us, IsWritable: true},
			{Pubkey: ProtocolTreasuryPDA(a.ProgramID), IsWritable: true},
			{Pubkey: vault, IsWritable: true},
			{Pubkey: vaultSol, IsWritable: true},
			{Pubkey: link},
			{Pubkey: vaultToken, IsWritable: true},
			{Pubkey: Token2022Program},
			{Pubkey: SystemProgram},
			{Pubkey: TorchEventAuthorityPDA(a.ProgramID)},
			{Pubkey: a.ProgramID},
		},
		Data: data,
	}
	out := []sol.Instruction{ix}
	if memo != "" {
		m, err := BuildMemo(a.Buyer, memo)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// SwapAccounts holds everything a DeepPool vault swap needs.
type SwapAccounts struct {
	Mint         string
	Signer       string
	VaultCreator string
	ProgramID    string
	DeepPoolID   string
}

// BuildVaultSwap builds vault_swap (DeepPool CPI) from the IDL: discriminator,
// account order, borsh (u64 amount_in, u64 minimum_amount_out, bool is_buy).
func BuildVaultSwap(idlIx []byte, a SwapAccounts, amountIn, minimumOut uint64, isBuy bool, memo string, withATA bool) ([]sol.Instruction, error) {
	vault := TorchVaultPDA(a.ProgramID, a.VaultCreator)
	vaultSol := VaultSolPDA(a.ProgramID, a.VaultCreator)
	link := VaultWalletLinkPDA(a.ProgramID, a.Signer)
	vaultToken, err := ATA(a.Mint, vault, Token2022Program)
	if err != nil {
		return nil, err
	}
	torchConfig := TorchConfigPDA(a.ProgramID)
	pool := DeepPoolPDA(a.DeepPoolID, torchConfig, a.Mint)
	poolVault := DeepPoolVaultPDA(a.DeepPoolID, pool)
	data := append(append([]byte{}, idlIx...), leU64(amountIn)...)
	data = append(data, leU64(minimumOut)...)
	if isBuy {
		data = append(data, 1)
	} else {
		data = append(data, 0)
	}
	ix := sol.Instruction{
		ProgramID: a.ProgramID,
		Accounts: []sol.AccountMeta{
			{Pubkey: a.Signer, IsSigner: true, IsWritable: true},
			{Pubkey: vault, IsWritable: true},
			{Pubkey: vaultSol, IsWritable: true},
			{Pubkey: link},
			{Pubkey: a.Mint, IsWritable: true},
			{Pubkey: BondingCurvePDA(a.ProgramID, a.Mint)},
			{Pubkey: vaultToken, IsWritable: true},
			{Pubkey: a.DeepPoolID},
			{Pubkey: pool, IsWritable: true},
			{Pubkey: poolVault, IsWritable: true},
			{Pubkey: DeepPoolEventAuthorityPDA(a.DeepPoolID)},
			{Pubkey: Token2022Program},
			{Pubkey: SystemProgram},
			{Pubkey: TorchEventAuthorityPDA(a.ProgramID)},
			{Pubkey: a.ProgramID},
		},
		Data: data,
	}
	out := []sol.Instruction{}
	if withATA {
		out = append(out, BuildVaultATAIx(a.Signer, a.Mint, vault, vaultToken))
	}
	out = append(out, ix)
	if memo != "" {
		m, err := BuildMemo(a.Signer, memo)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func leU64(n uint64) []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(n >> (8 * i))
	}
	return b
}
