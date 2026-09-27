package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/sol"
)

const (
	SideLongByte  byte = 0
	SideShortByte byte = 1

	FullRepayBPS uint16 = 10_000

	LendingUnlockLamports uint64 = 1_000_000_000

	positionIndexBound uint32 = 256
)

type PositionAccounts struct {
	Mint         string
	Signer       string
	VaultCreator string
	ProgramID    string
	DeepPoolID   string
}

func (a PositionAccounts) keys(side byte, index uint32) (map[string]string, error) {
	vault := TorchVaultPDA(a.ProgramID, a.VaultCreator)
	position := PositionPDA(a.ProgramID, vault, a.Mint, side, index)
	vaultToken, err := ATA(a.Mint, vault, Token2022Program)
	if err != nil {
		return nil, err
	}
	positionToken, err := ATA(a.Mint, position, Token2022Program)
	if err != nil {
		return nil, err
	}
	lockToken, err := ATA(a.Mint, TreasuryLockPDA(a.ProgramID, a.Mint), Token2022Program)
	if err != nil {
		return nil, err
	}
	pool := DeepPoolPDA(a.DeepPoolID, TorchConfigPDA(a.ProgramID), a.Mint)
	return map[string]string{
		"signer":                      a.Signer,
		"torch_vault":                 vault,
		"vault_sol":                   VaultSolPDA(a.ProgramID, a.VaultCreator),
		"vault_wallet_link":           VaultWalletLinkPDA(a.ProgramID, a.Signer),
		"mint":                        a.Mint,
		"treasury":                    TokenTreasuryPDA(a.ProgramID, a.Mint),
		"treasury_sol_vault":          TreasurySolVaultPDA(a.ProgramID, a.Mint),
		"treasury_lock":               TreasuryLockPDA(a.ProgramID, a.Mint),
		"treasury_lock_token_account": lockToken,
		"vault_token_account":         vaultToken,
		"position":                    position,
		"user_risk":                   UserRiskPDA(a.ProgramID, vault, a.Mint),
		"position_token_vault":        positionToken,
		"long_sol_vault":              LongSolVaultPDA(a.ProgramID, vault, a.Mint, index),
		"position_sol_vault":          ShortVaultPDA(a.ProgramID, vault, a.Mint, index),
		"deep_pool_program":           a.DeepPoolID,
		"deep_pool":                   pool,
		"deep_pool_token_vault":       DeepPoolVaultPDA(a.DeepPoolID, pool),
		"deep_pool_event_authority":   DeepPoolEventAuthorityPDA(a.DeepPoolID),
		"token_2022_program":          Token2022Program,
		"associated_token_program":    ATProgram,
		"system_program":              SystemProgram,
		"event_authority":             TorchEventAuthorityPDA(a.ProgramID),
		"program":                     a.ProgramID,
	}, nil
}

func BuildFromIDL(id *idl.IDL, program, name string, keys map[string]string, args map[string]any) (sol.Instruction, error) {
	ins, ok := id.Instructions[name]
	if !ok {
		return sol.Instruction{}, fmt.Errorf("build %s: not in the IDL", name)
	}
	data, err := id.BorshArgs(name, args)
	if err != nil {
		return sol.Instruction{}, fmt.Errorf("build %s: %w", name, err)
	}
	metas := make([]sol.AccountMeta, 0, len(ins.Accounts))
	for _, acc := range ins.Accounts {
		pk, ok := keys[acc.Name]
		if !ok {
			return sol.Instruction{}, fmt.Errorf("build %s: no key for account %s", name, acc.Name)
		}
		metas = append(metas, sol.AccountMeta{Pubkey: pk, IsSigner: acc.Signer, IsWritable: acc.Writable})
	}
	disc := make([]byte, 8)
	copy(disc, ins.Discriminator)
	return sol.Instruction{ProgramID: program, Accounts: metas, Data: append(disc, data...)}, nil
}

func positionIxName(side PositionSide, open bool) (string, byte, error) {
	switch {
	case side == SideLong && open:
		return "open_long_via_vault", SideLongByte, nil
	case side == SideLong:
		return "close_long_via_vault", SideLongByte, nil
	case side == SideShort && open:
		return "open_short_via_vault", SideShortByte, nil
	case side == SideShort:
		return "close_short_via_vault", SideShortByte, nil
	}
	return "", 0, fmt.Errorf("position: unknown side %q", side)
}

func BuildPosition(id *idl.IDL, a PositionAccounts, w PositionWrite) ([]sol.Instruction, error) {
	name, sideByte, err := positionIxName(w.Side, w.Open)
	if err != nil {
		return nil, err
	}
	keys, err := a.keys(sideByte, w.Index)
	if err != nil {
		return nil, err
	}
	var args map[string]any
	if w.Open {
		args = map[string]any{"position_index": w.Index, "collateral": w.Amount, "min_out": w.MinOut}
	} else {
		args = map[string]any{"position_index": w.Index, "repay_fraction_bps": w.RepayBPS, "min_surplus_sol_out": w.MinOut}
	}
	ix, err := BuildFromIDL(id, a.ProgramID, name, keys, args)
	if err != nil {
		return nil, err
	}
	out := []sol.Instruction{ix}
	if w.Memo != "" {
		m, err := BuildMemo(a.Signer, w.Memo)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

type PositionWrite struct {
	Side     PositionSide
	Open     bool
	Index    uint32
	Amount   uint64
	MinOut   uint64
	RepayBPS uint16
	Memo     string
}

func (c *TorchClient) WritePosition(ctx context.Context, market MarketRow, w PositionWrite) (WriteResult, error) {
	if !c.AllowWrite {
		return WriteResult{}, errors.New("write: disabled (devnet gate off)")
	}
	if market.Status != StatusMigrated {
		return WriteResult{}, fmt.Errorf("position: %s is not public (status %s); leverage opens only after migration", market.Mint, market.Status)
	}
	if w.Open && w.Amount == 0 {
		return WriteResult{}, errors.New("position: open needs collateral")
	}
	if !w.Open && (w.RepayBPS == 0 || w.RepayBPS > FullRepayBPS) {
		return WriteResult{}, fmt.Errorf("position: repay fraction %d bps outside (0, %d]", w.RepayBPS, FullRepayBPS)
	}
	name, _, err := positionIxName(w.Side, w.Open)
	if err != nil {
		return WriteResult{}, err
	}
	ixs, err := BuildPosition(c.IDL, PositionAccounts{
		Mint: market.Mint, Signer: c.AgentPublic(), VaultCreator: c.VaultCreator,
		ProgramID: c.ProgramID, DeepPoolID: DeepPoolProgramID,
	}, w)
	if err != nil {
		return WriteResult{}, err
	}
	return c.send(ctx, ixs, name, w.Memo, w.Amount, w.MinOut)
}

func (c *TorchClient) NextPositionIndex(ctx context.Context, mint string, side PositionSide) (uint32, error) {
	sideByte := SideLongByte
	if side == SideShort {
		sideByte = SideShortByte
	}
	vault := c.VaultPDA()
	for i := uint32(0); i < positionIndexBound; i++ {
		info, err := c.RPC.GetAccountInfo(ctx, PositionPDA(c.ProgramID, vault, mint, sideByte, i))
		if err != nil {
			return 0, err
		}
		if !info.Exists {
			return i, nil
		}
	}
	return 0, fmt.Errorf("position: no free index under %d on %s", positionIndexBound, mint)
}

func (c *TorchClient) OpenPositions(ctx context.Context, mint string, side PositionSide) ([]PositionRow, error) {
	return c.API.Positions(ctx, Q("mint", mint, "owner", c.VaultPDA(), "side", string(side), "is_active", "true"))
}

type Carrier struct {
	Lamports   uint64
	Long       bool
	Short      bool
	Vault      string
	Index      uint32
	Collateral uint64
}

func CarrierOf(id *idl.IDL, program string, tx *Transaction) Carrier {
	var c Carrier
	if id == nil || tx == nil || tx.Err {
		return c
	}
	ixs := append(append([]TxInstruction{}, tx.Ixs...), tx.InnerIxs...)
	for _, ix := range ixs {
		if ix.ProgramID != program || len(ix.Data) < 8 {
			continue
		}
		switch {
		case discIs(id, "buy_via_vault", ix.Data) && len(ix.Data) >= 16:
			c.Lamports += binary.LittleEndian.Uint64(ix.Data[8:16])
		case discIs(id, "vault_swap", ix.Data) && len(ix.Data) >= 25:
			if ix.Data[24] != 0 {
				c.Lamports += binary.LittleEndian.Uint64(ix.Data[8:16])
			}
		case discIs(id, "open_long_via_vault", ix.Data) && len(ix.Data) >= 20:
			c.Long = true
			c.Index = binary.LittleEndian.Uint32(ix.Data[8:12])
			c.Collateral = binary.LittleEndian.Uint64(ix.Data[12:20])
			c.Vault = accountAt(ix, 1)
		case discIs(id, "open_short_via_vault", ix.Data) && len(ix.Data) >= 20:
			c.Short = true
			c.Index = binary.LittleEndian.Uint32(ix.Data[8:12])
			c.Collateral = binary.LittleEndian.Uint64(ix.Data[12:20])
			c.Vault = accountAt(ix, 1)
		}
	}
	return c
}

func discIs(id *idl.IDL, name string, data []byte) bool {
	d, err := id.Discriminator(name)
	if err != nil {
		return false
	}
	return bytesEqual(d, data[:8])
}

func accountAt(ix TxInstruction, i int) string {
	if i < len(ix.Accounts) {
		return ix.Accounts[i]
	}
	return ""
}

func (c *TorchClient) Carrier(ctx context.Context, signature string) (Carrier, error) {
	tx, err := c.RPC.GetTransaction(ctx, signature)
	if err != nil {
		return Carrier{}, err
	}
	return CarrierOf(c.IDL, c.ProgramID, tx), nil
}

type PositionKey struct {
	Vault string
	Index uint32
}

func (c *TorchClient) LongEnds(ctx context.Context, mint string) (map[PositionKey]string, error) {
	if c.Indexer == "" {
		return nil, nil
	}
	rows, err := c.API.Positions(ctx, Q("mint", mint, "side", string(SideLong), "limit", "500"))
	if err != nil {
		return nil, err
	}
	events, err := c.API.PositionEvents(ctx, Q("mint", mint, "side", string(SideLong), "limit", "500"))
	if err != nil {
		return nil, err
	}
	active := map[PositionKey]bool{}
	for _, r := range rows {
		if r.IsActive {
			active[PositionKey{Vault: r.Owner, Index: uint32(r.PositionIndex)}] = true
		}
	}
	ends := map[PositionKey]string{}
	for _, ev := range events {
		key := PositionKey{Vault: ev.Owner, Index: uint32(ev.PositionIndex)}
		if ev.Kind == "open" || active[key] {
			continue
		}
		if prev, ok := ends[key]; !ok || ev.CreatedAt > prev {
			ends[key] = ev.CreatedAt
		}
	}
	return ends, nil
}
