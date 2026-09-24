package client

import (
	"encoding/hex"
	"testing"

	"github.com/mrsirg97-rgb/orbit/idl"
)

func loadIDL(t *testing.T) *idl.IDL {
	t.Helper()
	id, err := idl.LoadIDL()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBuyViaVaultGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "d52ef036cd132719" // IDL 21.0.0
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}
	args, err := id.BorshArgs("buy_via_vault", map[string]any{
		"sol_amount":     uint64(10_000_000),
		"min_tokens_out": uint64(9_900_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := hex.EncodeToString(append(append([]byte{}, disc...), args...))
	want := "d52ef036cd132719" + "8096980000000000" + "e00f970000000000"
	if got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
}

func TestSellViaVaultGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("sell_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "ce4753215361e23b"
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}
	args, err := id.BorshArgs("sell_via_vault", map[string]any{
		"token_amount": uint64(1_000_000_000),
		"min_sol_out":  uint64(148_024),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := hex.EncodeToString(append(append([]byte{}, disc...), args...))
	want := "ce4753215361e23b" + "00ca9a3b00000000" + "3842020000000000"
	if got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
}

func TestVaultSwapGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("vault_swap")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "8fc29a8ae6deed1c"
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}
	// amount_in 10_000_000, minimum 9_000_000, is_buy true.
	data := append(append([]byte{}, disc...), leU64(10_000_000)...)
	data = append(data, leU64(9_000_000)...)
	data = append(data, 1)
	want := "8fc29a8ae6deed1c" + "8096980000000000" + "4054890000000000" + "01"
	if hex.EncodeToString(data) != want {
		t.Fatalf("data %x, want %s", data, want)
	}
}

func TestBuyAccountOrderAndPDAs(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	const buyer = "So11111111111111111111111111111111111111112"
	const creator = "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh"
	ixs, err := BuildBuyViaVault(disc, BuyAccounts{
		Mint: mint, Creator: creator, DevWallet: buyer,
		Buyer: buyer, VaultCreator: buyer, ProgramID: DevnetProgramID, WithATA: true,
	}, 10_000_000, 9_900_000, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ixs) != 2 { // ATA create + buy
		t.Fatalf("got %d instructions, want 2", len(ixs))
	}
	buy := ixs[1]
	if buy.ProgramID != DevnetProgramID {
		t.Errorf("program: %s", buy.ProgramID)
	}
	names, err := id.AccountNames("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	if len(buy.Accounts) != len(names) {
		t.Fatalf("accounts %d, IDL names %d", len(buy.Accounts), len(names))
	}
	wantFirst := []string{"buyer", "global_config", "dev_wallet", "mint", "bonding_curve"}
	for i, want := range wantFirst {
		if buy.Accounts[i].Pubkey == "" {
			t.Fatalf("account %d empty", i)
		}
		_ = want
	}
	if !buy.Accounts[0].IsSigner || !buy.Accounts[0].IsWritable {
		t.Error("buyer must be signer + writable")
	}
	// PDA spot checks against the SDK-verified values.
	bc := BondingCurvePDA(DevnetProgramID, mint)
	if bc != "6wDUn9V7fuP1Ujn6o3xk4yFh4EE3F65LpgQsrNjTjmVx" {
		t.Errorf("bonding curve PDA: %s", bc)
	}
	if buy.Accounts[4].Pubkey != bc {
		t.Errorf("bonding_curve account: %s", buy.Accounts[4].Pubkey)
	}
	vault := TorchVaultPDA(DevnetProgramID, buyer)
	if vault != "HkPCW8mFF6ndNGzKufyfsYLcmpjTaGVKaoDAjwjBauh4" {
		t.Errorf("vault PDA: %s", vault)
	}
	if buy.Accounts[14].Pubkey != vault {
		t.Errorf("torch_vault account: %s", buy.Accounts[14].Pubkey)
	}
	vaultATA, err := ATA(mint, vault, Token2022Program)
	if err != nil {
		t.Fatal(err)
	}
	if vaultATA != "2xYXam7tjfmLHu2HDNQzx2aiiTFp24Nie5Bv5y3jTmSp" {
		t.Errorf("vault ATA: %s", vaultATA)
	}
	if buy.Accounts[17].Pubkey != vaultATA {
		t.Errorf("vault_token_account: %s", buy.Accounts[17].Pubkey)
	}
	// The ATA-create instruction precedes the buy and is idempotent ([1]).
	if ixs[0].ProgramID != ATProgram {
		t.Errorf("ATA program: %s", ixs[0].ProgramID)
	}
	if len(ixs[0].Data) != 1 || ixs[0].Data[0] != 1 {
		t.Errorf("ATA data: %x", ixs[0].Data)
	}
}

func TestVaultSwapAccountOrder(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("vault_swap")
	if err != nil {
		t.Fatal(err)
	}
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	const signer = "So11111111111111111111111111111111111111112"
	ixs, err := BuildVaultSwap(disc, SwapAccounts{
		Mint: mint, Signer: signer, VaultCreator: signer,
		ProgramID: DevnetProgramID, DeepPoolID: DeepPoolProgramID,
	}, 10_000_000, 9_000_000, true, "memo", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ixs) != 3 { // ATA create + swap + memo
		t.Fatalf("got %d instructions, want 3", len(ixs))
	}
	swap := ixs[1]
	if !swap.Accounts[0].IsSigner {
		t.Error("signer must be signer")
	}
	pool := DeepPoolPDA(DeepPoolProgramID, TorchConfigPDA(DevnetProgramID), mint)
	if pool != "H5uVsoK2KydBthZYgMhh5Z2mJBRNfr7ZCZpLiVb9zjCf" {
		t.Errorf("deep pool PDA: %s", pool)
	}
	if swap.Accounts[8].Pubkey != pool {
		t.Errorf("deep_pool account: %s", swap.Accounts[8].Pubkey)
	}
	memo := ixs[2]
	if memo.ProgramID != MemoProgram {
		t.Errorf("memo program: %s", memo.ProgramID)
	}
	if string(memo.Data) != "memo" {
		t.Errorf("memo data: %q", string(memo.Data))
	}
}

func TestMemoCap(t *testing.T) {
	long := make([]rune, 501)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := BuildMemo("wallet", string(long)); err != nil {
		t.Fatalf("memo builder must not cap (the callers enforce the path cap): %v", err)
	}
}
