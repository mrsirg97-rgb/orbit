package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mrsirg97-rgb/orbit/sol"
)

func ScanMessages(ctx context.Context, rpc RPC, programID, mint string, limit int) ([]MessageRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	curve := BondingCurvePDA(programID, mint)
	sigs, err := rpc.GetSignaturesForAddress(ctx, curve, limit)
	if err != nil {
		return nil, fmt.Errorf("scan %s: signatures: %w", mint, err)
	}
	rows := make([]MessageRow, 0, len(sigs))
	seen := map[string]bool{}
	for _, s := range sigs {
		if s.Signature == "" || seen[s.Signature] {
			continue
		}
		seen[s.Signature] = true
		tx, err := rpc.GetTransaction(ctx, s.Signature)
		if err != nil {
			return nil, fmt.Errorf("scan %s: transaction %s: %w", mint, s.Signature, err)
		}
		if tx.Err || len(tx.Keys) == 0 {
			continue
		}
		memo, found := memoOf(tx)
		if !found {
			continue
		}
		createdAt := time.Unix(0, 0)
		if tx.BlockTime != nil {
			createdAt = time.Unix(*tx.BlockTime, 0)
		}
		rows = append(rows, MessageRow{
			MessageID:  int32(len(rows) + 1),
			Mint:       mint,
			Sender:     tx.Keys[0],
			MemoText:   memo,
			ActionKind: actionKindOf(tx),
			Slot:       tx.Slot,
			Signature:  s.Signature,
			CreatedAt:  createdAt.UTC().Format(time.RFC3339),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Slot != rows[j].Slot {
			return rows[i].Slot < rows[j].Slot
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt < rows[j].CreatedAt
		}
		return rows[i].Signature < rows[j].Signature
	})
	return rows, nil
}

func memoOf(tx *Transaction) (string, bool) {
	for _, ix := range tx.Ixs {
		if ix.ProgramID == MemoProgram {
			return string(ix.Data), true
		}
	}
	for _, ix := range tx.InnerIxs {
		if ix.ProgramID == MemoProgram {
			return string(ix.Data), true
		}
	}
	return "", false
}

func actionKindOf(tx *Transaction) *string {
	kind := ""
	for _, ix := range tx.Ixs {
		if k, ok := actionKindFor(ix); ok {
			kind = k
		}
	}
	for _, ix := range tx.InnerIxs {
		if k, ok := actionKindFor(ix); ok {
			kind = k
		}
	}
	if kind == "" {
		return nil
	}
	return &kind
}

func actionKindFor(ix TxInstruction) (string, bool) {
	if len(ix.Data) < 8 {
		return "", false
	}
	disc := ix.Data[:8]
	buyDisc := []byte{102, 6, 61, 18, 1, 218, 235, 234}
	buyVaultDisc := []byte{213, 46, 240, 54, 205, 19, 39, 25}
	sellDisc := []byte{51, 230, 133, 164, 1, 127, 131, 173}
	sellVaultDisc := []byte{206, 71, 83, 33, 83, 97, 226, 59}
	swapDisc := []byte{143, 194, 154, 138, 230, 222, 237, 28}
	switch {
	case bytesEqual(disc, buyDisc), bytesEqual(disc, buyVaultDisc):
		return "buy", true
	case bytesEqual(disc, sellDisc), bytesEqual(disc, sellVaultDisc):
		return "sell", true
	case bytesEqual(disc, swapDisc):

		if len(ix.Data) >= 25 {
			if ix.Data[24] != 0 {
				return "buy", true
			}
			return "sell", true
		}
		return "", false
	}
	return "", false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type BondingCurveState struct {
	Mint                string
	Creator             string
	VirtualSol          uint64
	VirtualToken        uint64
	RealSol             uint64
	RealToken           uint64
	BondingComplete     bool
	BondingCompleteSlot uint64
	Migrated            bool
	LastActivitySlot    uint64
	Reclaimed           bool
	BondingTarget       uint64
}

func DecodeBondingCurve(data []byte) (BondingCurveState, error) {
	if len(data) < 8+32+32+8+8+8+8+1+8+1+8+1+1+1+8 {
		return BondingCurveState{}, errors.New("bonding curve: account data too short")
	}
	b := data[8:]
	adv := func(n int) []byte {
		out := b[:n]
		b = b[n:]
		return out
	}
	var s BondingCurveState
	s.Mint = sol.Encode(adv(32))
	s.Creator = sol.Encode(adv(32))
	s.VirtualSol = binary.LittleEndian.Uint64(adv(8))
	s.VirtualToken = binary.LittleEndian.Uint64(adv(8))
	s.RealSol = binary.LittleEndian.Uint64(adv(8))
	s.RealToken = binary.LittleEndian.Uint64(adv(8))
	s.BondingComplete = adv(1)[0] != 0
	s.BondingCompleteSlot = binary.LittleEndian.Uint64(adv(8))
	s.Migrated = adv(1)[0] != 0
	s.LastActivitySlot = binary.LittleEndian.Uint64(adv(8))
	s.Reclaimed = adv(1)[0] != 0
	adv(1)
	adv(1)
	s.BondingTarget = binary.LittleEndian.Uint64(adv(8))
	return s, nil
}

func MarketFromRPC(ctx context.Context, rpc RPC, programID, mint string) (MarketRow, error) {
	info, err := rpc.GetAccountInfo(ctx, BondingCurvePDA(programID, mint))
	if err != nil {
		return MarketRow{}, fmt.Errorf("market rpc %s: curve: %w", mint, err)
	}
	if !info.Exists {
		return MarketRow{}, fmt.Errorf("market rpc %s: bonding curve missing", mint)
	}
	curve, err := DecodeBondingCurve(info.Data)
	if err != nil {
		return MarketRow{}, err
	}
	if curve.Mint != mint {
		return MarketRow{}, fmt.Errorf("market rpc %s: curve mint %s mismatch", mint, curve.Mint)
	}
	row := MarketRow{
		Mint:         mint,
		Creator:      curve.Creator,
		VirtualSol:   int64(curve.VirtualSol),
		VirtualToken: int64(curve.VirtualToken),
		RealSol:      int64(curve.RealSol),
		RealToken:    int64(curve.RealToken),
		SolTarget:    int64(curve.BondingTarget),
		Status:       StatusBonding,
	}
	switch {
	case curve.Reclaimed:
		row.Status = StatusReclaimed
	case curve.Migrated:
		row.Status = StatusMigrated
	case curve.BondingComplete:
		row.Status = StatusComplete
	}
	if t, err := rpc.GetAccountInfo(ctx, TokenTreasuryPDA(programID, mint)); err == nil && t.Exists && len(t.Data) >= 8+32+32+1 {
		row.IsCommunityToken = t.Data[8+32+32] != 0
	}
	return row, nil
}
