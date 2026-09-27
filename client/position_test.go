package client

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
)

func TestPositionBuildersAgainstIDL(t *testing.T) {
	id := loadIDL(t)
	const hot = "4we9cx8o8YoLR9QxyhKiUGrMx2QyCrH8rkpnGXqQwktm"
	const creator = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	a := PositionAccounts{Mint: mint, Signer: hot, VaultCreator: creator, ProgramID: vaultTestProgram, DeepPoolID: DeepPoolProgramID}
	vault := TorchVaultPDA(vaultTestProgram, creator)
	for _, c := range []struct {
		w     PositionWrite
		name  string
		side  byte
		args  []byte
		count int
	}{
		{PositionWrite{Side: SideLong, Open: true, Index: 2, Amount: 5_000_000, MinOut: 7}, "open_long_via_vault", SideLongByte, append(append(leU32(2), leU64(5_000_000)...), leU64(7)...), 20},
		{PositionWrite{Side: SideLong, Index: 2, RepayBPS: FullRepayBPS, MinOut: 9}, "close_long_via_vault", SideLongByte, append(append(leU32(2), 0x10, 0x27), leU64(9)...), 19},
		{PositionWrite{Side: SideShort, Open: true, Index: 0, Amount: 3_000_000, MinOut: 1}, "open_short_via_vault", SideShortByte, append(append(leU32(0), leU64(3_000_000)...), leU64(1)...), 20},
		{PositionWrite{Side: SideShort, Index: 0, RepayBPS: 5_000, MinOut: 0}, "close_short_via_vault", SideShortByte, append(append(leU32(0), 0x88, 0x13), leU64(0)...), 19},
	} {
		ixs, err := BuildPosition(id, a, c.w)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		ix := ixs[0]
		if len(ixs) != 1 {
			t.Errorf("%s: %d instructions without a memo, want 1", c.name, len(ixs))
		}
		if string(ix.Data[:8]) != string(wantDisc(t, c.name)) {
			t.Errorf("%s: discriminator %x", c.name, ix.Data[:8])
		}
		if string(ix.Data[8:]) != string(c.args) {
			t.Errorf("%s: args %x, want %x", c.name, ix.Data[8:], c.args)
		}
		if len(ix.Accounts) != c.count {
			t.Fatalf("%s: %d accounts, want %d", c.name, len(ix.Accounts), c.count)
		}
		names, err := id.AccountNames(c.name)
		if err != nil {
			t.Fatal(err)
		}
		byName := map[string]string{}
		for i, n := range names {
			byName[n] = ix.Accounts[i].Pubkey
		}
		if byName["signer"] != hot || !ix.Accounts[0].IsSigner || !ix.Accounts[0].IsWritable {
			t.Errorf("%s: signer meta %+v", c.name, ix.Accounts[0])
		}
		if byName["torch_vault"] != vault {
			t.Errorf("%s: torch_vault %s", c.name, byName["torch_vault"])
		}
		if byName["position"] != PositionPDA(vaultTestProgram, vault, mint, c.side, c.w.Index) {
			t.Errorf("%s: position %s", c.name, byName["position"])
		}
		if byName["user_risk"] != UserRiskPDA(vaultTestProgram, vault, mint) {
			t.Errorf("%s: user_risk %s", c.name, byName["user_risk"])
		}
		if c.side == SideLongByte && byName["long_sol_vault"] != LongSolVaultPDA(vaultTestProgram, vault, mint, c.w.Index) {
			t.Errorf("%s: long_sol_vault %s", c.name, byName["long_sol_vault"])
		}
		if c.side == SideShortByte && byName["position_sol_vault"] != ShortVaultPDA(vaultTestProgram, vault, mint, c.w.Index) {
			t.Errorf("%s: position_sol_vault %s", c.name, byName["position_sol_vault"])
		}
		if byName["deep_pool_program"] != DeepPoolProgramID || byName["program"] != vaultTestProgram {
			t.Errorf("%s: program accounts %s %s", c.name, byName["deep_pool_program"], byName["program"])
		}
	}
	withMemo, err := BuildPosition(id, a, PositionWrite{Side: SideLong, Open: true, Index: 1, Amount: 1, Memo: "claim 3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(withMemo) != 2 || withMemo[1].ProgramID != MemoProgram || string(withMemo[1].Data) != "claim 3" {
		t.Errorf("memo rides the open: %+v", withMemo)
	}
}

func TestPositionPDAsDifferBySideAndIndex(t *testing.T) {
	const vault = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	seen := map[string]bool{}
	for _, side := range []byte{SideLongByte, SideShortByte} {
		for _, index := range []uint32{0, 1, 2} {
			pk := PositionPDA(vaultTestProgram, vault, mint, side, index)
			if seen[pk] {
				t.Errorf("position PDA collides at side %d index %d", side, index)
			}
			seen[pk] = true
		}
	}
}

func TestCarrierReadsBuyLongAndShort(t *testing.T) {
	id := loadIDL(t)
	const hot = "4we9cx8o8YoLR9QxyhKiUGrMx2QyCrH8rkpnGXqQwktm"
	const creator = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	vault := TorchVaultPDA(vaultTestProgram, creator)
	a := PositionAccounts{Mint: mint, Signer: hot, VaultCreator: creator, ProgramID: vaultTestProgram, DeepPoolID: DeepPoolProgramID}
	toTx := func(ixs ...[]byte) *Transaction {
		tx := &Transaction{Keys: []string{hot}}
		for _, d := range ixs {
			tx.Ixs = append(tx.Ixs, TxInstruction{ProgramID: vaultTestProgram, Accounts: []string{hot, vault}, Data: d})
		}
		return tx
	}
	buy := append(append(wantDisc(t, "buy_via_vault"), leU64(25_000_000)...), leU64(1)...)
	c := CarrierOf(id, vaultTestProgram, toTx(buy))
	if c.Lamports != 25_000_000 || c.Long || c.Short {
		t.Errorf("buy carrier: %+v", c)
	}
	swapBuy := append(append(append(wantDisc(t, "vault_swap"), leU64(4_000_000)...), leU64(0)...), 1)
	swapSell := append(append(append(wantDisc(t, "vault_swap"), leU64(9_000_000)...), leU64(0)...), 0)
	c = CarrierOf(id, vaultTestProgram, toTx(swapBuy, swapSell))
	if c.Lamports != 4_000_000 {
		t.Errorf("swap carrier counts only the buy: %+v", c)
	}
	long, err := BuildPosition(id, a, PositionWrite{Side: SideLong, Open: true, Index: 3, Amount: 777})
	if err != nil {
		t.Fatal(err)
	}
	c = CarrierOf(id, vaultTestProgram, toTx(long[0].Data))
	if !c.Long || c.Short || c.Index != 3 || c.Collateral != 777 || c.Vault != vault {
		t.Errorf("long carrier: %+v", c)
	}
	short, err := BuildPosition(id, a, PositionWrite{Side: SideShort, Open: true, Index: 1, Amount: 55})
	if err != nil {
		t.Fatal(err)
	}
	c = CarrierOf(id, vaultTestProgram, toTx(short[0].Data))
	if !c.Short || c.Long || c.Index != 1 || c.Collateral != 55 {
		t.Errorf("short carrier: %+v", c)
	}
	failed := toTx(buy)
	failed.Err = true
	if c := CarrierOf(id, vaultTestProgram, failed); c.Lamports != 0 {
		t.Errorf("a failed tx carries nothing: %+v", c)
	}
	if c := CarrierOf(id, vaultTestProgram, nil); c != (Carrier{}) {
		t.Errorf("a missing tx carries nothing: %+v", c)
	}
}

func TestWritePositionRefusesBeforeMigration(t *testing.T) {
	rpc := &fakeRPC{blockhash: "11111111111111111111111111111111"}
	tc := testClient(t, rpc)
	market := tc.API.(*stubAPI).market
	market.Status = StatusBonding
	_, err := tc.WritePosition(context.Background(), market, PositionWrite{Side: SideLong, Open: true, Amount: 1})
	if err == nil || !strings.Contains(err.Error(), "not public") {
		t.Fatalf("open on a bonding market: %v", err)
	}
	if rpc.sent != "" {
		t.Error("a refused open must not send")
	}
	market.Status = StatusMigrated
	res, err := tc.WritePosition(context.Background(), market, PositionWrite{Side: SideLong, Open: true, Index: 0, Amount: 1_000, Memo: "claim 1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "open_long_via_vault" || res.Memo != "claim 1" {
		t.Errorf("result: %+v", res)
	}
	parts := parseTx(t, rpc.sent)
	var found bool
	for _, ix := range parts.Ixs {
		if ix.ProgramID == DevnetProgramID && string(ix.Data[:8]) == string(wantDisc(t, "open_long_via_vault")) {
			found = true
			if binary.LittleEndian.Uint64(ix.Data[12:20]) != 1_000 {
				t.Errorf("collateral %x", ix.Data[12:20])
			}
		}
	}
	if !found {
		t.Error("no open_long_via_vault in the sent tx")
	}
	_, err = tc.WritePosition(context.Background(), market, PositionWrite{Side: SideLong, Index: 0, RepayBPS: 0})
	if err == nil || !strings.Contains(err.Error(), "repay fraction") {
		t.Errorf("zero repay: %v", err)
	}
}

func TestNextPositionIndexSkipsExisting(t *testing.T) {
	rpc := &fakeRPC{blockhash: "11111111111111111111111111111111", accounts: map[string]AccountInfo{}}
	tc := testClient(t, rpc)
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	for i := uint32(0); i < 2; i++ {
		rpc.accounts[PositionPDA(tc.ProgramID, tc.VaultPDA(), mint, SideLongByte, i)] = AccountInfo{Exists: true}
	}
	next, err := tc.NextPositionIndex(context.Background(), mint, SideLong)
	if err != nil {
		t.Fatal(err)
	}
	if next != 2 {
		t.Errorf("next long index %d, want 2", next)
	}
	next, err = tc.NextPositionIndex(context.Background(), mint, SideShort)
	if err != nil {
		t.Fatal(err)
	}
	if next != 0 {
		t.Errorf("next short index %d, want 0", next)
	}
}
